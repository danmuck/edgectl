package ghost

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
	seedcatalog "github.com/danmuck/edgectl/internal/seeds/catalog"
	seeddocker "github.com/danmuck/edgectl/internal/seeds/docker"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	seedfs "github.com/danmuck/edgectl/internal/seeds/fs"
	seedhost "github.com/danmuck/edgectl/internal/seeds/host"
	seedkv "github.com/danmuck/edgectl/internal/seeds/kv"
	seedmongod "github.com/danmuck/edgectl/internal/seeds/mongod"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInvalidHeartbeatInterval = errors.New("ghost: invalid heartbeat interval")
	ErrUnknownBuiltinSeed       = errors.New("ghost: unknown builtin seed")
	ErrInvalidMiragePolicy      = errors.New("ghost: invalid mirage session policy")
)

// MirageSessionPolicy controls Ghost behavior when Mirage is unavailable.
type MirageSessionPolicy string

const (
	MiragePolicyHeadless MirageSessionPolicy = "headless"
	MiragePolicyAuto     MirageSessionPolicy = "auto"
	MiragePolicyRequired MirageSessionPolicy = "required"
)

// MirageSessionConfig configures optional Ghost<->Mirage session behavior.
type MirageSessionConfig struct {
	Policy             MirageSessionPolicy
	Address            string
	PeerIdentity       string
	MaxConnectAttempts int
	SessionConfig      session.Config
}

// Ghost seed-install configuration for whitelist-gated dependency installation.
type SeedInstallConfig struct {
	Enabled       bool
	WorkspaceRoot string
	InstallRoot   string
	BinRoot       string
	Whitelist     []string
	Specs         []seeds.InstallSpec
}

// ServiceConfig configures Ghost standalone runtime defaults.
type ServiceConfig struct {
	GhostID             string
	ProjectRoot         string
	ProjectFetchOnBoot  bool
	BuiltinSeedIDs      []string
	SeedInstall         SeedInstallConfig
	HeartbeatInterval   time.Duration
	AdminListenAddr     string
	AdminFrameAuthToken string
	EnableClusterHost   bool
	Mirage              MirageSessionConfig
}

// Ghost service defaults for standalone runtime configuration.
func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		GhostID:             "ghost.local",
		ProjectRoot:         "",
		ProjectFetchOnBoot:  true,
		BuiltinSeedIDs:      []string{"seed.flow"},
		SeedInstall:         SeedInstallConfig{Enabled: false, InstallRoot: filepath.Join("local", "seeds")},
		HeartbeatInterval:   5 * time.Second,
		AdminListenAddr:     "",
		AdminFrameAuthToken: "",
		EnableClusterHost:   true,
		Mirage: MirageSessionConfig{
			Policy:        MiragePolicyHeadless,
			SessionConfig: session.DefaultConfig(),
		},
	}
}

// Service runs the Ghost server lifecycle as a standalone process.
type Service struct {
	server *Server
	cfg    ServiceConfig
	mu     sync.RWMutex
	mirage *MirageSession
	seq    atomic.Uint64

	mirageAdminBound   atomic.Bool
	mirageResolvedAddr atomic.Value  // string: resolved mirage session address from bind_mirage
	mirageBindNotify   chan struct{} // signaled when bind_mirage resolves the session address
	mirageClientMu     sync.RWMutex
	mirageActiveClient *MirageClient // current connect/register client for live bind_mirage address updates

	adminMu                 sync.Mutex
	adminSeq                atomic.Uint64
	adminEvents             []EventEnv
	verificationEvents      []VerificationRecord
	adminFrameAuthValidator CommandFrameAuthValidator
	adminClientCount        atomic.Int64
	cluster                 clusterHost
}

// CommandFrameAuthValidator validates optional command-frame auth bytes for admin execute_envelope RPCs.
type CommandFrameAuthValidator func(auth []byte) error

// Ghost service constructor using default standalone config.
func NewService() *Service {
	return NewServiceWithConfig(DefaultServiceConfig())
}

// Ghost service constructor using explicit config.
func NewServiceWithConfig(cfg ServiceConfig) *Service {
	cfg.Mirage.SessionConfig = cfg.Mirage.SessionConfig.WithDefaults()
	if strings.TrimSpace(string(cfg.Mirage.Policy)) == "" {
		cfg.Mirage.Policy = MiragePolicyHeadless
	}
	return &Service{
		server:                  NewServer(),
		cfg:                     cfg,
		adminEvents:             make([]EventEnv, 0),
		verificationEvents:      make([]VerificationRecord, 0),
		adminFrameAuthValidator: defaultCommandFrameAuthValidator(strings.TrimSpace(cfg.AdminFrameAuthToken)),
		cluster:                 newClusterHost(),
		mirageBindNotify:        make(chan struct{}, 1),
	}
}

// SetAdminCommandFrameAuthValidator overrides admin execute_envelope auth validation behavior.
func (s *Service) SetAdminCommandFrameAuthValidator(validator CommandFrameAuthValidator) {
	if validator == nil {
		validator = defaultCommandFrameAuthValidator(strings.TrimSpace(s.cfg.AdminFrameAuthToken))
	}
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.adminFrameAuthValidator = validator
}

// Ghost runtime entrypoint that blocks until process signal shutdown.
func (s *Service) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := s.bootstrap(); err != nil {
		return err
	}
	return s.serve(ctx)
}

// Ghost runtime accessor for lifecycle/execution boundary owner.
func (s *Service) Server() *Server {
	return s.server
}

// Ghost bootstrap sequence: appear->seed->radiate lifecycle transitions.
func (s *Service) bootstrap() error {
	if s.cfg.HeartbeatInterval <= 0 {
		return ErrInvalidHeartbeatInterval
	}
	if err := validateMiragePolicy(s.cfg.Mirage.Policy); err != nil {
		return err
	}
	if err := s.fetchProjectRepoOnBoot(); err != nil {
		logs.Warnf("ghost.Service.bootstrap project fetch skipped err=%v", err)
	}
	if err := s.installSeedDependencies(); err != nil {
		return err
	}

	if err := s.server.Appear(GhostConfig{GhostID: s.cfg.GhostID}); err != nil {
		return err
	}

	reg, err := buildBuiltinRegistry(s.cfg.BuiltinSeedIDs, s.cfg.GhostID)
	if err != nil {
		return err
	}
	if err := s.server.Seed(reg); err != nil {
		return err
	}
	if err := s.server.Radiate(); err != nil {
		return err
	}

	status := s.server.Status()
	logs.Infof(
		"ghost.Service.bootstrap ready ghost_id=%q phase=%s seeds=%d",
		status.GhostID,
		status.Phase,
		status.SeedCount,
	)
	return nil
}

// Ghost main loop for heartbeat logging and optional Mirage session supervision.
func (s *Service) serve(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()
	defer s.clearMirageSession()
	defer s.stopManagedGhosts()

	sessionErr := make(chan error, 1)
	controlErr := make(chan error, 1)
	if s.cfg.Mirage.Policy != MiragePolicyHeadless {
		go func() {
			sessionErr <- s.runMirageSessionLoop(ctx)
		}()
	}
	if strings.TrimSpace(s.cfg.AdminListenAddr) != "" {
		go func() {
			controlErr <- s.serveAdminControl(ctx, s.cfg.AdminListenAddr)
		}()
	}

	for {
		select {
		case <-ctx.Done():
			logs.Infof("ghost.Service.serve shutdown")
			return nil
		case err := <-sessionErr:
			if err != nil {
				return err
			}
		case err := <-controlErr:
			if err != nil {
				return err
			}
		case <-ticker.C:
			status := s.server.Status()
			adminClients := s.AdminClientCount()
			mirageConnected := s.IsMirageConnected()
			mirageLink := s.MirageLinkMode()
			managedChildren := s.ManagedGhostCount()
			hostSummary := s.fetchHostSummary()
			logs.Infof(
				"ghost.Service.heartbeat ghost_id=%q phase=%s seeds=%d mirage_connected=%v mirage_link=%q admin_clients=%d managed_children=%d host=%q",
				status.GhostID,
				status.Phase,
				status.SeedCount,
				mirageConnected,
				mirageLink,
				adminClients,
				managedChildren,
				hostSummary,
			)
		}
	}
}

// Ghost-side Mirage session manager with reconnect behavior.
func (s *Service) runMirageSessionLoop(ctx context.Context) error {
	attempt := 0
	connectedOnce := false
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		sessionConn, err := s.connectMirageSession(ctx)
		if err != nil {
			if errors.Is(err, ErrMirageAddressRequired) || errors.Is(err, ErrGhostIDRequired) {
				if s.cfg.Mirage.Policy == MiragePolicyRequired && !connectedOnce {
					return err
				}
				logs.Warnf("ghost.Service.runMirageSessionLoop disabled err=%v", err)
				return nil
			}
			if s.cfg.Mirage.Policy == MiragePolicyRequired && !connectedOnce {
				return err
			}
			attempt++

			// Slow-wait mode: conserve energy when mirage has never been reachable.
			const slowWaitThreshold = 10
			const slowWaitInterval = 30 * time.Second
			if !connectedOnce && attempt >= slowWaitThreshold {
				if attempt == slowWaitThreshold {
					logs.Warnf("ghost.Service.runMirageSessionLoop entering slow-wait mode policy=%q", s.cfg.Mirage.Policy)
				}
				// Log every 6th attempt (~3 min at 30s interval) to reduce noise.
				if attempt%6 == 0 {
					logs.Warnf("ghost.Service.runMirageSessionLoop slow-wait attempt=%d policy=%q", attempt, s.cfg.Mirage.Policy)
				}
				slowTimer := time.NewTimer(slowWaitInterval)
				select {
				case <-ctx.Done():
					slowTimer.Stop()
					return ctx.Err()
				case <-s.mirageBindNotify:
					slowTimer.Stop()
					logs.Warnf("ghost.Service.runMirageSessionLoop bind_mirage received, waking from slow-wait")
				case <-slowTimer.C:
				}
				continue
			}

			logs.Warnf(
				"ghost.Service.runMirageSessionLoop connect failed attempt=%d policy=%q err=%v",
				attempt,
				s.cfg.Mirage.Policy,
				err,
			)
			if err := s.waitReconnectBackoff(ctx, attempt); err != nil {
				return err
			}
			continue
		}
		attempt = 0
		connectedOnce = true
		s.setMirageSession(sessionConn)
		resolvedAddr, _ := s.mirageResolvedAddr.Load().(string)
		logs.Warnf(
			"ghost.Service.runMirageSessionLoop connected policy=%q address=%q",
			s.cfg.Mirage.Policy,
			resolvedAddr,
		)

		err = s.monitorMirageSession(ctx, sessionConn)
		s.clearMirageSessionIf(sessionConn)
		if err != nil && ctx.Err() == nil {
			logs.Warnf("ghost.Service.runMirageSessionLoop session lost err=%v", err)
		}
	}
}

// Ghost Mirage client dial/register wrapper using runtime seed metadata.
func (s *Service) connectMirageSession(ctx context.Context) (*MirageSession, error) {
	seedList := SeedInfoFromMetadata(s.server.SeedMetadata())
	// Enrich seed.host description with live host summary if available.
	hostSummary := s.fetchHostSummary()
	if hostSummary != "" {
		for i := range seedList {
			if seedList[i].ID == "seed.host" {
				desc := strings.TrimSpace(seedList[i].Description)
				if desc == "" {
					desc = hostSummary
				} else {
					desc = desc + "; " + hostSummary
				}
				seedList[i].Description = desc
			}
		}
	}
	// Prefer resolved mirage session address from bind_mirage over configured hostname.
	mirageAddr := strings.TrimSpace(s.cfg.Mirage.Address)
	if resolved, ok := s.mirageResolvedAddr.Load().(string); ok && resolved != "" {
		mirageAddr = resolved
	}
	clientCfg := MirageClientConfig{
		Address:            mirageAddr,
		GhostID:            strings.TrimSpace(s.cfg.GhostID),
		PeerIdentity:       strings.TrimSpace(s.cfg.Mirage.PeerIdentity),
		SeedList:           seedList,
		Session:            s.cfg.Mirage.SessionConfig,
		MaxConnectAttempts: s.cfg.Mirage.MaxConnectAttempts,
	}

	client, err := NewMirageClient(clientCfg)
	if err != nil {
		return nil, err
	}
	// Allow bind_mirage to update an in-flight connect/register client.
	s.setActiveMirageClient(client)
	defer s.clearActiveMirageClientIf(client)

	sessionConn, err := client.ConnectAndRegister(ctx)
	if err != nil {
		return nil, err
	}
	// Lock in the working address so all future reconnects use it.
	s.mirageResolvedAddr.Store(client.CurrentAddress())
	return sessionConn, nil
}

// setActiveMirageClient records the current connect/register client for live address updates.
func (s *Service) setActiveMirageClient(client *MirageClient) {
	s.mirageClientMu.Lock()
	defer s.mirageClientMu.Unlock()
	s.mirageActiveClient = client
}

// clearActiveMirageClientIf clears the active client only when pointer identity matches.
func (s *Service) clearActiveMirageClientIf(client *MirageClient) {
	s.mirageClientMu.Lock()
	defer s.mirageClientMu.Unlock()
	if s.mirageActiveClient != client {
		return
	}
	s.mirageActiveClient = nil
}

// activeMirageClient returns the current connect/register client, if one is in flight.
func (s *Service) activeMirageClient() *MirageClient {
	s.mirageClientMu.RLock()
	defer s.mirageClientMu.RUnlock()
	return s.mirageActiveClient
}

// Ghost session health probe loop using heartbeat events.
func (s *Service) monitorMirageSession(ctx context.Context, conn *MirageSession) error {
	interval := s.cfg.Mirage.SessionConfig.HeartbeatInterval
	if interval <= 0 {
		interval = s.cfg.HeartbeatInterval
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			probeCtx, cancel := context.WithTimeout(ctx, s.sessionProbeTimeout())
			_, err := conn.SendEventWithAck(probeCtx, s.sessionProbeEvent())
			cancel()
			if err != nil {
				return err
			}
		}
	}
}

// Ghost runtime atomic swap for the active Mirage session pointer.
func (s *Service) setMirageSession(conn *MirageSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != nil && s.mirage != conn {
		_ = s.mirage.Close()
	}
	s.mirage = conn
}

// Ghost runtime session cleanup that closes and clears active Mirage session.
func (s *Service) clearMirageSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != nil {
		_ = s.mirage.Close()
		s.mirage = nil
	}
}

// Ghost runtime guarded session clear that requires pointer identity match.
func (s *Service) clearMirageSessionIf(target *MirageSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != target {
		return
	}
	_ = s.mirage.Close()
	s.mirage = nil
}

// Ghost runtime accessor for current Mirage session pointer, if any.
func (s *Service) MirageSession() *MirageSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mirage
}

// IsMirageConnected reports whether Ghost currently has an active Mirage session.
func (s *Service) IsMirageConnected() bool {
	s.mu.RLock()
	hasTransport := s.mirage != nil
	s.mu.RUnlock()
	return hasTransport || s.mirageAdminBound.Load()
}

// MirageLinkMode reports how Ghost is currently attached to Mirage.
func (s *Service) MirageLinkMode() string {
	s.mu.RLock()
	hasTransport := s.mirage != nil
	s.mu.RUnlock()
	if hasTransport {
		return "session"
	}
	if s.mirageAdminBound.Load() {
		return "admin_route"
	}
	return "none"
}

// BindMirageAdminRoute marks this Ghost as connected to Mirage via admin routing.
// If remoteAddr and sessionPort are provided, ghost resolves the mirage session dial target
// by extracting the IP from the admin connection and combining it with the session port.
func (s *Service) BindMirageAdminRoute(mirageID string, remoteAddr string, sessionPort string) {
	id := strings.TrimSpace(mirageID)
	s.mirageAdminBound.Store(true)

	port := strings.TrimSpace(sessionPort)
	remote := strings.TrimSpace(remoteAddr)
	if port != "" && remote != "" {
		host, _, err := net.SplitHostPort(remote)
		if err == nil && host != "" {
			resolved := net.JoinHostPort(host, port)
			s.mirageResolvedAddr.Store(resolved)
			// Update any in-flight connect/register client immediately.
			if client := s.activeMirageClient(); client != nil {
				if err := client.UpdateAddress(resolved); err != nil {
					logs.Warnf("ghost.admin mirage route update ignored mirage_id=%q err=%v", id, err)
				}
			}
			// Wake the session loop so it retries with the resolved address immediately.
			select {
			case s.mirageBindNotify <- struct{}{}:
			default:
			}
			logs.Warnf("ghost.admin mirage route bound mirage_id=%q resolved_session_addr=%q", id, resolved)
			return
		}
	}
	logs.Warnf("ghost.admin mirage route bound mirage_id=%q", id)
}

// AdminClientCount returns the current number of attached admin control clients.
func (s *Service) AdminClientCount() int64 {
	return s.adminClientCount.Load()
}

// ManagedGhostCount returns number of currently managed child Ghost nodes.
func (s *Service) ManagedGhostCount() int {
	return s.cluster.count()
}

// Ghost session probe timeout derived from session config.
func (s *Service) sessionProbeTimeout() time.Duration {
	if s.cfg.Mirage.SessionConfig.SessionDeadAfter > 0 {
		return s.cfg.Mirage.SessionConfig.SessionDeadAfter
	}
	if s.cfg.Mirage.SessionConfig.AckTimeout > 0 {
		return s.cfg.Mirage.SessionConfig.AckTimeout
	}
	return 5 * time.Second
}

// Ghost synthetic heartbeat event builder for Mirage session probes.
func (s *Service) sessionProbeEvent() EventEnv {
	now := uint64(time.Now().UnixMilli())
	seq := s.seq.Add(1)
	return EventEnv{
		EventID:     fmt.Sprintf("evt.session.%s.%d", s.cfg.GhostID, seq),
		CommandID:   fmt.Sprintf("cmd.session.heartbeat.%d", seq),
		IntentID:    "intent.session.heartbeat",
		GhostID:     s.cfg.GhostID,
		SeedID:      "seed.session",
		Outcome:     OutcomeSuccess,
		TimestampMS: now,
	}
}

// Ghost reconnect backoff wait helper with deterministic delay.
// Wakes early if bind_mirage delivers a resolved address.
func (s *Service) waitReconnectBackoff(ctx context.Context, attempt int) error {
	backoffCfg := s.cfg.Mirage.SessionConfig.Backoff
	backoffCfg.Jitter = false
	delay := session.NextBackoffDelay(backoffCfg, attempt, nil)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-s.mirageBindNotify:
		logs.Warnf("ghost.Service.waitReconnectBackoff bind_mirage received, retrying immediately")
		return nil
	case <-timer.C:
		return nil
	}
}

// Ghost policy validator for allowed Mirage session policy values.
func validateMiragePolicy(policy MirageSessionPolicy) error {
	switch policy {
	case MiragePolicyHeadless, MiragePolicyAuto, MiragePolicyRequired:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMiragePolicy, policy)
	}
}

// Ghost builtin-seed resolver that builds a runtime registry.
func buildBuiltinRegistry(seedIDs []string, ghostID string) (*seeds.Registry, error) {
	reg := seeds.NewRegistry()
	localGhostID := strings.TrimSpace(ghostID)
	if localGhostID == "" {
		localGhostID = "ghost.local"
	}

	seen := make(map[string]struct{})
	for _, raw := range seedIDs {
		id := strings.TrimSpace(raw)
		if id == "" || id == "none" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		switch id {
		case "seed.flow", "flow":
			if err := reg.Register(seedflow.NewSeed()); err != nil {
				return nil, err
			}
		case "seed.mongod", "mongod":
			if err := reg.Register(seedmongod.NewSeed()); err != nil {
				return nil, err
			}
		case "seed.kv", "kv":
			if err := reg.Register(seedkv.NewSeed()); err != nil {
				return nil, err
			}
		case "seed.fs", "fs":
			// Seed.fs uses a ghost-scoped root to avoid collisions on shared hosts.
			root := filepath.Join("local", "dir", localGhostID)
			if err := reg.Register(seedfs.NewSeedWithRoot(root)); err != nil {
				return nil, err
			}
		case "seed.docker", "docker":
			if err := reg.Register(seeddocker.NewSeed()); err != nil {
				return nil, err
			}
		case "seed.host", "host":
			if err := reg.Register(seedhost.NewSeed()); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownBuiltinSeed, id)
		}
	}

	return reg, nil
}

// Ghost bootstrap hook that runs configured seed dependency installations.
// When no explicit specs are configured, resolves deps from the catalog for
// each enabled builtin seed — so a Ghost can auto-resolve its seed dependencies.
func (s *Service) installSeedDependencies() error {
	cfg := s.cfg.SeedInstall
	if !cfg.Enabled {
		return nil
	}

	specs := cfg.Specs
	if len(specs) == 0 {
		specs = s.specsFromCatalog()
	}
	if len(specs) == 0 {
		return nil
	}

	installer, err := seeds.NewInstaller(seeds.InstallerConfig{
		WorkspaceRoot: cfg.WorkspaceRoot,
		InstallRoot:   cfg.InstallRoot,
		BinRoot:       cfg.BinRoot,
		Whitelist:     cfg.Whitelist,
	})
	if err != nil {
		return err
	}
	if err := installer.InstallAll(specs); err != nil {
		return err
	}
	return nil
}

// specsFromCatalog derives InstallSpecs from the dependency catalog for each configured builtin seed.
func (s *Service) specsFromCatalog() []seeds.InstallSpec {
	catalog := seedcatalog.DependencyCatalog()
	var specs []seeds.InstallSpec
	for _, rawID := range s.cfg.BuiltinSeedIDs {
		seedID := strings.TrimSpace(rawID)
		// Normalize short IDs to canonical form.
		if !strings.HasPrefix(seedID, "seed.") {
			seedID = "seed." + seedID
		}
		deps, ok := catalog[seedID]
		if !ok {
			continue
		}
		for _, dep := range deps {
			specs = append(specs, seeds.InstallSpec{
				SeedID:  seedID + ".dep." + dep.Name,
				Method:  dep.InstallMethod,
				Package: dep.AptPackage,
			})
		}
	}
	return specs
}

// fetchHostSummary queries seed.host status if registered and returns a compact summary string.
func (s *Service) fetchHostSummary() string {
	seed, ok := s.server.ResolveSeed("seed.host")
	if !ok {
		return ""
	}
	result, err := seed.Execute("status", nil)
	if err != nil || result.Status != "ok" {
		return ""
	}
	// Compact multi-line output into semicolon-separated fields.
	lines := strings.Split(strings.TrimSpace(string(result.Stdout)), "\n")
	var parts []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "; ")
}

// Ghost bootstrap hook that refreshes root project refs before seed install.
func (s *Service) fetchProjectRepoOnBoot() error {
	if !s.cfg.ProjectFetchOnBoot {
		return nil
	}

	projectRoot := strings.TrimSpace(s.cfg.ProjectRoot)
	if projectRoot == "" {
		projectRoot = strings.TrimSpace(s.cfg.SeedInstall.WorkspaceRoot)
	}
	if projectRoot == "" {
		return nil
	}

	projectAbs, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(projectAbs, ".git")); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	runner := tools.ExecRunner{}
	stdout, stderr, exitCode, err := runner.Run("git", "-C", projectAbs, "remote")
	if err != nil {
		return fmt.Errorf(
			"ghost: git remote failed root=%q exit=%d stdout=%q stderr=%q: %w",
			projectAbs,
			exitCode,
			strings.TrimSpace(string(stdout)),
			strings.TrimSpace(string(stderr)),
			err,
		)
	}
	if strings.TrimSpace(string(stdout)) == "" {
		logs.Infof("ghost.Service.fetchProjectRepoOnBoot skip root=%q reason=no_remotes", projectAbs)
		return nil
	}

	stdout, stderr, exitCode, err = runner.Run("git", "-C", projectAbs, "fetch", "--all", "--prune")
	if err != nil {
		return fmt.Errorf(
			"ghost: git fetch failed root=%q exit=%d stdout=%q stderr=%q: %w",
			projectAbs,
			exitCode,
			strings.TrimSpace(string(stdout)),
			strings.TrimSpace(string(stderr)),
			err,
		)
	}
	logs.Infof("ghost.Service.fetchProjectRepoOnBoot ok root=%q", projectAbs)
	return nil
}
