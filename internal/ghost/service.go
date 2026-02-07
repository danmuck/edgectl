package ghost

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInvalidHeartbeatInterval = errors.New("ghost: invalid heartbeat interval")
	ErrUnknownBuiltinSeed       = errors.New("ghost: unknown builtin seed")
	ErrInvalidMiragePolicy      = errors.New("ghost: invalid mirage session policy")
)

type MirageSessionPolicy string

const (
	MiragePolicyHeadless MirageSessionPolicy = "headless"
	MiragePolicyAuto     MirageSessionPolicy = "auto"
	MiragePolicyRequired MirageSessionPolicy = "required"
)

type MirageSessionConfig struct {
	Policy             MirageSessionPolicy
	Address            string
	PeerIdentity       string
	MaxConnectAttempts int
	SessionConfig      session.Config
}

type ServiceConfig struct {
	GhostID           string
	BuiltinSeedIDs    []string
	HeartbeatInterval time.Duration
	Mirage            MirageSessionConfig
}

func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		GhostID:           "ghost.local",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 5 * time.Second,
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
}

// NewService creates a Ghost service with default standalone config.
func NewService() *Service {
	return NewServiceWithConfig(DefaultServiceConfig())
}

// NewServiceWithConfig creates a Ghost service with explicit config.
func NewServiceWithConfig(cfg ServiceConfig) *Service {
	cfg.Mirage.SessionConfig = cfg.Mirage.SessionConfig.WithDefaults()
	if strings.TrimSpace(string(cfg.Mirage.Policy)) == "" {
		cfg.Mirage.Policy = MiragePolicyHeadless
	}
	return &Service{
		server: NewServer(),
		cfg:    cfg,
	}
}

// Run starts Ghost lifecycle and blocks until process signal shutdown.
func (s *Service) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := s.bootstrap(); err != nil {
		return err
	}
	return s.serve(ctx)
}

// Server returns the lifecycle/execution boundary owner for Ghost.
func (s *Service) Server() *Server {
	return s.server
}

func (s *Service) bootstrap() error {
	if s.cfg.HeartbeatInterval <= 0 {
		return ErrInvalidHeartbeatInterval
	}
	if err := validateMiragePolicy(s.cfg.Mirage.Policy); err != nil {
		return err
	}

	if err := s.server.Appear(GhostConfig{GhostID: s.cfg.GhostID}); err != nil {
		return err
	}

	reg, err := buildBuiltinRegistry(s.cfg.BuiltinSeedIDs)
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

func (s *Service) serve(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()
	defer s.clearMirageSession()

	sessionErr := make(chan error, 1)
	if s.cfg.Mirage.Policy != MiragePolicyHeadless {
		go func() {
			sessionErr <- s.runMirageSessionLoop(ctx)
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
		case <-ticker.C:
			status := s.server.Status()
			logs.Infof(
				"ghost.Service.heartbeat ghost_id=%q phase=%s seeds=%d",
				status.GhostID,
				status.Phase,
				status.SeedCount,
			)
		}
	}
}

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
		logs.Infof(
			"ghost.Service.runMirageSessionLoop connected policy=%q address=%q",
			s.cfg.Mirage.Policy,
			s.cfg.Mirage.Address,
		)

		err = s.monitorMirageSession(ctx, sessionConn)
		s.clearMirageSessionIf(sessionConn)
		if err != nil && ctx.Err() == nil {
			logs.Warnf("ghost.Service.runMirageSessionLoop session lost err=%v", err)
		}
	}
}

func (s *Service) connectMirageSession(ctx context.Context) (*MirageSession, error) {
	clientCfg := MirageClientConfig{
		Address:            strings.TrimSpace(s.cfg.Mirage.Address),
		GhostID:            strings.TrimSpace(s.cfg.GhostID),
		PeerIdentity:       strings.TrimSpace(s.cfg.Mirage.PeerIdentity),
		SeedList:           SeedInfoFromMetadata(s.server.SeedMetadata()),
		Session:            s.cfg.Mirage.SessionConfig,
		MaxConnectAttempts: s.cfg.Mirage.MaxConnectAttempts,
	}

	client, err := NewMirageClient(clientCfg)
	if err != nil {
		return nil, err
	}

	sessionConn, err := client.ConnectAndRegister(ctx)
	if err != nil {
		return nil, err
	}
	return sessionConn, nil
}

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

func (s *Service) setMirageSession(conn *MirageSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != nil && s.mirage != conn {
		_ = s.mirage.Close()
	}
	s.mirage = conn
}

func (s *Service) clearMirageSession() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != nil {
		_ = s.mirage.Close()
		s.mirage = nil
	}
}

func (s *Service) clearMirageSessionIf(target *MirageSession) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.mirage != target {
		return
	}
	_ = s.mirage.Close()
	s.mirage = nil
}

func (s *Service) MirageSession() *MirageSession {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mirage
}

func (s *Service) sessionProbeTimeout() time.Duration {
	if s.cfg.Mirage.SessionConfig.SessionDeadAfter > 0 {
		return s.cfg.Mirage.SessionConfig.SessionDeadAfter
	}
	if s.cfg.Mirage.SessionConfig.AckTimeout > 0 {
		return s.cfg.Mirage.SessionConfig.AckTimeout
	}
	return 5 * time.Second
}

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

func (s *Service) waitReconnectBackoff(ctx context.Context, attempt int) error {
	backoffCfg := s.cfg.Mirage.SessionConfig.Backoff
	backoffCfg.Jitter = false
	delay := session.NextBackoffDelay(backoffCfg, attempt, nil)
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func validateMiragePolicy(policy MirageSessionPolicy) error {
	switch policy {
	case MiragePolicyHeadless, MiragePolicyAuto, MiragePolicyRequired:
		return nil
	default:
		return fmt.Errorf("%w: %q", ErrInvalidMiragePolicy, policy)
	}
}

func buildBuiltinRegistry(seedIDs []string) (*seeds.Registry, error) {
	reg := seeds.NewRegistry()

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
			if err := reg.Register(seeds.NewFlowSeed()); err != nil {
				return nil, err
			}
		case "seed.mongod", "mongod":
			if err := reg.Register(seeds.NewMongodSeed()); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownBuiltinSeed, id)
		}
	}

	return reg, nil
}
