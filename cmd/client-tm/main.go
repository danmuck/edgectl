package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	seedmongod "github.com/danmuck/edgectl/internal/seeds/mongod"
	logs "github.com/danmuck/smplog"
)

const (
	ghostConfigPath  = "cmd/client-tm/ghost.toml"
	mirageConfigPath = "cmd/client-tm/mirage.toml"
)

var (
	// ErrNavigateBack signals caller-intent to return to the previous menu.
	ErrNavigateBack = errors.New("navigate back")
	// ErrNavigateExit signals caller-intent to exit the interactive client.
	ErrNavigateExit = errors.New("navigate exit")
)

// ghostConfigFile persists Ghost targets configured for the client.
type ghostConfigFile struct {
	ClearScreenAfterCommand bool                `toml:"clear_screen_after_command"`
	Targets                 []ghostTargetConfig `toml:"targets"`
}

// ghostTargetConfig binds a display name to a Ghost admin endpoint.
type ghostTargetConfig struct {
	Name    string `toml:"name"`
	Addr    string `toml:"addr"`
	GhostID string `toml:"ghost_id"`
}

// mirageConfigFile persists one Mirage control-plane target plus local Ghost linkage.
type mirageConfigFile struct {
	Targets             []mirageTargetConfig `toml:"targets"`
	LocalGhostID        string               `toml:"local_ghost_id"`
	LocalGhostAdminAddr string               `toml:"local_ghost_admin_addr"`
}

type mirageTargetConfig struct {
	Name     string `toml:"name"`
	Addr     string `toml:"addr"`
	MirageID string `toml:"mirage_id"`
}

// GhostAdminCommand is the input envelope for admin-triggered Ghost command execution.
type GhostAdminCommand struct {
	IntentID     string            `json:"intent_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
}

// GhostAdmin defines the client control boundary for one Ghost target.
type GhostAdmin interface {
	GhostID() string
	Address() string
	Status() (ghost.LifecycleStatus, error)
	ListSeeds() ([]seeds.SeedMetadata, error)
	Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error)
	ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error)
	RecentEvents(limit int) ([]ghost.EventEnv, error)
	Verification(limit int) ([]ghost.VerificationRecord, error)
	SpawnGhost(req ghost.SpawnGhostRequest) (ghost.SpawnGhostResult, error)
	Close() error
}

// RemoteGhostAdmin is a TCP client for ghostctl admin control endpoint.
type RemoteGhostAdmin struct {
	addr string
	conn net.Conn
	r    *bufio.Reader
}

// GhostTarget maps a friendly name to a concrete Ghost admin implementation.
type GhostTarget struct {
	Name  string
	Admin GhostAdmin
}

// MirageAdmin defines the client control boundary for one Mirage target.
type MirageAdmin interface {
	Address() string
	Status() (mirage.LifecycleStatus, error)
	SubmitIssue(issue MirageIssueRequest) error
	ReconcileIntent(intentID string) (session.Report, error)
	ReconcileAll() ([]session.Report, error)
	SnapshotIntent(intentID string) (mirage.IntentSnapshot, bool, error)
	ListIntents() ([]string, error)
	RecentReports(limit int) ([]session.Report, error)
	SpawnLocalGhost(req mirage.SpawnGhostRequest) (mirage.SpawnGhostResult, error)
	RegisteredGhosts() ([]mirage.RegisteredGhost, error)
	Close() error
}

// RemoteMirageAdmin is a TCP client for miragectl admin control endpoint.
type RemoteMirageAdmin struct {
	addr string
	conn net.Conn
	r    *bufio.Reader
}

// MirageTarget maps a friendly name to a concrete Mirage admin implementation.
type MirageTarget struct {
	Name  string
	Admin MirageAdmin
}

// controlRequest is one line-delimited control request payload.
type controlRequest struct {
	Action    string                  `json:"action"`
	Limit     int                     `json:"limit,omitempty"`
	CommandID string                  `json:"command_id,omitempty"`
	Command   GhostAdminCommand       `json:"command,omitempty"`
	Spawn     ghost.SpawnGhostRequest `json:"spawn,omitempty"`
}

// controlResponse is one line-delimited control response payload.
type controlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// MirageIssueCommand defines one command step for mirage issue submission.
type MirageIssueCommand struct {
	GhostID      string            `json:"ghost_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
	Blocking     bool              `json:"blocking"`
}

// MirageIssueRequest defines one issue ingress payload for mirage admin control.
type MirageIssueRequest struct {
	IntentID    string               `json:"intent_id"`
	Actor       string               `json:"actor"`
	TargetScope string               `json:"target_scope"`
	Objective   string               `json:"objective"`
	CommandPlan []MirageIssueCommand `json:"command_plan"`
}

type mirageControlRequest struct {
	Action   string                   `json:"action"`
	Limit    int                      `json:"limit,omitempty"`
	IntentID string                   `json:"intent_id,omitempty"`
	Issue    MirageIssueRequest       `json:"issue,omitempty"`
	Spawn    mirage.SpawnGhostRequest `json:"spawn,omitempty"`
}

type mirageControlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type mirageSnapshotIntentResponse struct {
	Found    bool                  `json:"found"`
	Snapshot mirage.IntentSnapshot `json:"snapshot"`
}

type mirageReconcileAllResponse struct {
	Reports []session.Report `json:"reports"`
}

// executionResponse holds execute action output.
type executionResponse struct {
	Execution ghost.ExecutionState `json:"execution"`
	Event     ghost.EventEnv       `json:"event"`
}

// executionLookupResponse holds execution lookup output.
type executionLookupResponse struct {
	Found     bool                 `json:"found"`
	Execution ghost.ExecutionState `json:"execution"`
}

// App hosts interactive state and persisted target references.
type App struct {
	reader        *bufio.Reader
	ghostCfgPath  string
	mirageCfgPath string
	ghostCfg      ghostConfigFile
	mirageCfg     mirageConfigFile
	targets       []GhostTarget
	activeTarget  int
	mirageTargets []MirageTarget
	activeMirage  int
	clearScreen   bool
	launchMode    string
}

func main() {
	var mode string
	flag.StringVar(&mode, "mode", "ghost", "client mode: ghost or mirage")
	flag.Parse()

	logging.ConfigureRuntime()
	app := NewApp(ghostConfigPath, mirageConfigPath, mode)
	if err := app.Run(); err != nil {
		logs.Errf("client-tm: %v", err)
		os.Exit(1)
	}
}

func NewApp(ghostCfgPath string, mirageCfgPath string, mode string) *App {
	return &App{
		reader:        bufio.NewReader(os.Stdin),
		ghostCfgPath:  ghostCfgPath,
		mirageCfgPath: mirageCfgPath,
		targets:       make([]GhostTarget, 0),
		activeTarget:  -1,
		mirageTargets: make([]MirageTarget, 0),
		activeMirage:  -1,
		clearScreen:   false,
		launchMode:    strings.ToLower(strings.TrimSpace(mode)),
	}
}

// Run executes the main interactive menu loop.
func (a *App) Run() error {
	if err := a.loadOrInitConfigs(); err != nil {
		return err
	}
	logs.Infof(
		"client-tm loaded ghost_targets=%d mirage_targets=%d",
		len(a.ghostCfg.Targets),
		len(a.mirageCfg.Targets),
	)
	if a.launchMode != "" && a.launchMode != "ghost" && a.launchMode != "mirage" {
		return fmt.Errorf("invalid mode %q (expected ghost or mirage)", a.launchMode)
	}
	if a.launchMode == "mirage" {
		return a.runMirageClientLoop()
	}

	for {
		a.printMainMenu()
		choice, err := a.promptInt("Choose", 1, 10, false, true)
		if err != nil {
			if errors.Is(err, ErrNavigateExit) {
				return a.exitClient()
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.listTargets()
		case 2:
			if err := a.addGhostTarget(); err != nil {
				logs.Errf("add target failed: %v", err)
			}
		case 3:
			if err := a.selectActiveTarget(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("select target failed: %v", err)
			}
		case 4:
			a.showActiveTargetSummary()
		case 5:
			if err := a.runGhostAdminConsole(); err != nil {
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("ghost admin console error: %v", err)
			}
		case 6:
			if err := a.removeGhostTarget(); err != nil {
				if errors.Is(err, ErrNavigateBack) {
					continue
				}
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("remove target failed: %v", err)
			}
		case 7:
			a.clearScreen = !a.clearScreen
			a.ghostCfg.ClearScreenAfterCommand = a.clearScreen
			logs.Infof("clear_screen_after_command=%v", a.clearScreen)
		case 8:
			if err := a.resetToDefaultConfig(); err != nil {
				logs.Errf("reset config failed: %v", err)
			}
		case 9:
			if err := a.saveConfigs(); err != nil {
				logs.Errf("save failed: %v", err)
			} else {
				logs.Infof("config saved")
			}
		case 10:
			return a.exitClient()
		}
	}
}

// exitClient saves current config and closes active admin connections.
func (a *App) exitClient() error {
	if err := a.saveConfigs(); err != nil {
		logs.Warnf("save on exit failed: %v", err)
	}
	a.closeTargets()
	a.closeMirageTargets()
	logs.Infof("client-tm exiting")
	return nil
}

// loadOrInitConfigs loads persisted files and initializes runtime targets.
func (a *App) loadOrInitConfigs() error {
	if err := ensureFile(a.ghostCfgPath); err != nil {
		return err
	}
	if err := ensureFile(a.mirageCfgPath); err != nil {
		return err
	}

	if _, err := toml.DecodeFile(a.ghostCfgPath, &a.ghostCfg); err != nil {
		return fmt.Errorf("load ghost config: %w", err)
	}
	if _, err := toml.DecodeFile(a.mirageCfgPath, &a.mirageCfg); err != nil {
		return fmt.Errorf("load mirage config: %w", err)
	}
	a.clearScreen = a.ghostCfg.ClearScreenAfterCommand
	needsSave := false

	if len(a.ghostCfg.Targets) == 0 {
		a.ghostCfg.Targets = append(a.ghostCfg.Targets, ghostTargetConfig{
			Name:    "local-ghost",
			Addr:    "127.0.0.1:7010",
			GhostID: "ghost.local",
		})
		needsSave = true
	}
	if len(a.mirageCfg.Targets) == 0 {
		a.mirageCfg.Targets = append(a.mirageCfg.Targets, mirageTargetConfig{
			Name:     "local-mirage",
			Addr:     "127.0.0.1:7020",
			MirageID: "mirage.local",
		})
		needsSave = true
	}
	if len(a.mirageCfg.Targets) > 1 {
		logs.Warnf("client-tm mirage config has %d targets; only first target is supported", len(a.mirageCfg.Targets))
		a.mirageCfg.Targets = a.mirageCfg.Targets[:1]
		needsSave = true
	}
	if strings.TrimSpace(a.mirageCfg.LocalGhostID) == "" {
		a.mirageCfg.LocalGhostID = "ghost.local"
		needsSave = true
	}
	if strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr) == "" {
		a.mirageCfg.LocalGhostAdminAddr = "127.0.0.1:7010"
		needsSave = true
	}
	for i, cfg := range a.ghostCfg.Targets {
		name := strings.TrimSpace(cfg.Name)
		addr := strings.TrimSpace(cfg.Addr)
		if name == "" || addr == "" {
			continue
		}
		ghostID := strings.TrimSpace(cfg.GhostID)
		admin := NewRemoteGhostAdmin(addr)
		if ghostID == "" {
			if status, err := admin.Status(); err == nil && strings.TrimSpace(status.GhostID) != "" {
				ghostID = strings.TrimSpace(status.GhostID)
			} else {
				ghostID = inferGhostIDFromTargetName(name)
			}
			a.ghostCfg.Targets[i].GhostID = ghostID
			needsSave = true
		}
		a.targets = append(a.targets, GhostTarget{
			Name:  name,
			Admin: admin,
		})
	}
	if len(a.targets) > 0 {
		a.activeTarget = 0
	}
	cfg := a.mirageCfg.Targets[0]
	name := strings.TrimSpace(cfg.Name)
	addr := strings.TrimSpace(cfg.Addr)
	if name == "" || addr == "" {
		return errors.New("mirage config requires non-empty target name and addr")
	}
	mirageID := strings.TrimSpace(cfg.MirageID)
	admin := NewRemoteMirageAdmin(addr)
	if mirageID == "" {
		if status, err := admin.Status(); err == nil && strings.TrimSpace(status.MirageID) != "" {
			mirageID = strings.TrimSpace(status.MirageID)
		} else {
			mirageID = "mirage.local"
		}
		a.mirageCfg.Targets[0].MirageID = mirageID
		needsSave = true
	}
	a.mirageTargets = append(a.mirageTargets, MirageTarget{
		Name:  name,
		Admin: admin,
	})
	if len(a.mirageTargets) > 0 {
		a.activeMirage = 0
	}
	if needsSave {
		if err := a.saveConfigs(); err != nil {
			return err
		}
	}
	return nil
}

// saveConfigs writes current Ghost and Mirage target lists to disk.
func (a *App) saveConfigs() error {
	buf := strings.Builder{}
	if err := toml.NewEncoder(&buf).Encode(a.ghostCfg); err != nil {
		return err
	}
	if err := os.WriteFile(a.ghostCfgPath, []byte(buf.String()), 0o644); err != nil {
		return err
	}

	buf.Reset()
	if err := toml.NewEncoder(&buf).Encode(a.mirageCfg); err != nil {
		return err
	}
	if err := os.WriteFile(a.mirageCfgPath, []byte(buf.String()), 0o644); err != nil {
		return err
	}
	return nil
}

func (a *App) printMainMenu() {
	fmt.Println()
	fmt.Println("Client TM")
	fmt.Printf("  ghost config:  %s (targets=%d)\n", a.ghostCfgPath, len(a.ghostCfg.Targets))
	fmt.Printf("  mirage config: %s (targets=%d)\n", a.mirageCfgPath, len(a.mirageCfg.Targets))
	fmt.Printf("  clear screen after command: %v\n", a.clearScreen)
	fmt.Println("  1) List ghost targets")
	fmt.Println("  2) Add/provision ghost target (persist)")
	fmt.Println("  3) Select active ghost target")
	fmt.Println("  4) Show active target summary")
	fmt.Println("  5) Ghost admin console")
	fmt.Println("  6) Remove ghost target")
	fmt.Println("  7) Toggle clear-screen")
	fmt.Println("  8) Reset configs to defaults")
	fmt.Println("  9) Save configs")
	fmt.Println(" 10) Exit")
}

// runMirageClientLoop executes Mirage-first operator navigation.
func (a *App) runMirageClientLoop() error {
	for {
		a.printMirageMenu()
		choice, err := a.promptInt("Choose", 1, 8, false, true)
		if err != nil {
			if errors.Is(err, ErrNavigateExit) {
				return a.exitClient()
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.showMirageControlPlaneConfig()
		case 2:
			if err := a.showActiveMirageSummary(); err != nil {
				logs.Errf("show mirage summary failed: %v", err)
			}
		case 3:
			if err := a.runMirageAdminConsole(); err != nil {
				if errors.Is(err, ErrNavigateExit) {
					return a.exitClient()
				}
				logs.Errf("mirage admin console error: %v", err)
			}
		case 4:
			if err := a.showMirageConnectedGhosts(); err != nil {
				logs.Errf("show connected ghosts failed: %v", err)
			}
		case 5:
			if err := a.openLocalGhostConsole(); err != nil {
				logs.Errf("open local ghost console failed: %v", err)
			}
		case 6:
			a.clearScreen = !a.clearScreen
			a.ghostCfg.ClearScreenAfterCommand = a.clearScreen
			logs.Infof("clear_screen_after_command=%v", a.clearScreen)
		case 7:
			if err := a.saveConfigs(); err != nil {
				logs.Errf("save failed: %v", err)
			} else {
				logs.Infof("config saved")
			}
		case 8:
			return a.exitClient()
		}
	}
}

func (a *App) printMirageMenu() {
	fmt.Println()
	fmt.Println("Client TM (Mirage)")
	fmt.Printf("  ghost config:  %s (targets=%d)\n", a.ghostCfgPath, len(a.ghostCfg.Targets))
	fmt.Printf("  mirage config: %s (single control plane)\n", a.mirageCfgPath)
	fmt.Printf("  clear screen after command: %v\n", a.clearScreen)
	fmt.Println("  1) Show mirage control-plane config")
	fmt.Println("  2) Show mirage status")
	fmt.Println("  3) Mirage admin console")
	fmt.Println("  4) Show connected ghosts")
	fmt.Println("  5) Open local ghost admin console")
	fmt.Println("  6) Toggle clear-screen")
	fmt.Println("  7) Save configs")
	fmt.Println("  8) Exit")
}

func (a *App) activeMirageTarget() (MirageTarget, bool) {
	if a.activeMirage < 0 || a.activeMirage >= len(a.mirageTargets) {
		return MirageTarget{}, false
	}
	return a.mirageTargets[a.activeMirage], true
}

func (a *App) showMirageControlPlaneConfig() {
	target, ok := a.activeMirageTarget()
	if !ok {
		fmt.Println("No configured mirage target.")
		return
	}
	fmt.Println()
	fmt.Println("Mirage Control-Plane Config")
	fmt.Printf("  mirage name:        %s\n", target.Name)
	fmt.Printf("  mirage admin addr:  %s\n", target.Admin.Address())
	fmt.Printf("  local ghost id:     %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostID))
	fmt.Printf("  local ghost admin:  %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr))
}

func (a *App) showActiveMirageSummary() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	status, err := target.Admin.Status()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Active Mirage Target: %s\n", target.Name)
	fmt.Printf("  addr:      %s\n", target.Admin.Address())
	fmt.Printf("  mirage_id: %s\n", status.MirageID)
	fmt.Printf("  phase:     %s\n", status.Phase)
	fmt.Printf("  ghosts:    %d\n", status.RegisteredGhosts)
	fmt.Printf("  intents:   %d\n", status.ActiveIntents)
	fmt.Printf("  reports:   %d\n", status.ReportCount)
	fmt.Printf("  local_ghost_id:    %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostID))
	fmt.Printf("  local_ghost_admin: %s\n", strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr))
	return nil
}

func (a *App) showMirageConnectedGhosts() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	ghosts, err := target.Admin.RegisteredGhosts()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Connected Ghosts")
	if len(ghosts) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range ghosts {
		g := ghosts[i]
		fmt.Printf(
			"  [%d] ghost_id=%s connected=%v remote=%s seeds=%d events=%d\n",
			i+1,
			g.GhostID,
			g.Connected,
			g.RemoteAddr,
			len(g.SeedList),
			g.EventCount,
		)
	}
	return nil
}

// openLocalGhostConsole opens the local ghost admin console configured for this Mirage control plane.
func (a *App) openLocalGhostConsole() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	localGhostID := strings.TrimSpace(a.mirageCfg.LocalGhostID)
	localGhostAddr := strings.TrimSpace(a.mirageCfg.LocalGhostAdminAddr)
	if localGhostID == "" {
		return errors.New("mirage config local_ghost_id is required")
	}
	if localGhostAddr == "" {
		return errors.New("mirage config local_ghost_admin_addr is required")
	}
	logs.Infof(
		"opening local ghost admin console mirage=%q local_ghost_id=%q addr=%q",
		target.Name,
		localGhostID,
		localGhostAddr,
	)
	admin := NewRemoteGhostAdmin(localGhostAddr)
	defer admin.Close()
	return a.runGhostAdminConsoleForTarget(GhostTarget{
		Name:  localGhostID,
		Admin: admin,
	})
}

func (a *App) runMirageAdminConsole() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	for {
		fmt.Println()
		fmt.Printf("Mirage Admin Console (%s @ %s)\n", target.Name, target.Admin.Address())
		fmt.Println("  1) Show status")
		fmt.Println("  2) Submit issue")
		fmt.Println("  3) List intents")
		fmt.Println("  4) Reconcile one intent")
		fmt.Println("  5) Reconcile all intents")
		fmt.Println("  6) Snapshot intent")
		fmt.Println("  7) Show recent reports")
		fmt.Println("  8) Spawn local ghost")
		fmt.Println("  9) Back")
		choice, err := a.promptInt("Choose", 1, 9, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			if err := a.showActiveMirageSummary(); err != nil {
				logs.Errf("show mirage summary failed: %v", err)
			}
		case 2:
			if err := a.submitMirageIssue(target); err != nil {
				logs.Errf("submit issue failed: %v", err)
			}
		case 3:
			if err := a.listMirageIntents(target); err != nil {
				logs.Errf("list intents failed: %v", err)
			}
		case 4:
			if err := a.reconcileMirageIntent(target); err != nil {
				logs.Errf("reconcile intent failed: %v", err)
			}
		case 5:
			if err := a.reconcileAllMirageIntents(target); err != nil {
				logs.Errf("reconcile all failed: %v", err)
			}
		case 6:
			if err := a.snapshotMirageIntent(target); err != nil {
				logs.Errf("snapshot intent failed: %v", err)
			}
		case 7:
			if err := a.showMirageReports(target); err != nil {
				logs.Errf("show reports failed: %v", err)
			}
		case 8:
			if err := a.spawnMirageLocalGhost(target); err != nil {
				logs.Errf("spawn local ghost failed: %v", err)
			}
		case 9:
			return nil
		}
	}
}

func (a *App) listTargets() {
	fmt.Println()
	fmt.Println("Ghost Targets")
	if len(a.targets) == 0 {
		fmt.Println("  (none)")
		return
	}
	for i := range a.targets {
		target := a.targets[i]
		marker := " "
		if a.activeTarget == i {
			marker = "*"
		}
		status, err := target.Admin.Status()
		if err != nil {
			fmt.Printf("  %s [%d] %s addr=%s (status err: %v)\n", marker, i+1, target.Name, target.Admin.Address(), err)
			continue
		}
		fmt.Printf(
			"  %s [%d] %s addr=%s ghost_id=%s phase=%s seeds=%d\n",
			marker,
			i+1,
			target.Name,
			target.Admin.Address(),
			status.GhostID,
			status.Phase,
			status.SeedCount,
		)
	}
}

func (a *App) addGhostTarget() error {
	nameRaw, err := a.promptLine("Target suffix/name")
	if err != nil {
		return err
	}
	addrRaw, err := a.promptLine("Ghost admin addr (host:port)")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(nameRaw)
	addr := strings.TrimSpace(addrRaw)
	if name == "" || addr == "" {
		return errors.New("name and addr are required")
	}
	root, ok := a.active()
	if !ok {
		return errors.New("no active root ghost target selected")
	}
	rootStatus, err := root.Admin.Status()
	if err != nil {
		return fmt.Errorf("active root ghost unavailable: %w", err)
	}
	addr, err = normalizeTargetAddr(root.Admin.Address(), addr)
	if err != nil {
		return err
	}

	suffix := normalizeSuffix(name)
	targetName := normalizeSuffix(root.Name) + "." + suffix
	ghostID := rootStatus.GhostID + "." + suffix
	if a.targetExists(targetName, addr) {
		return fmt.Errorf("target exists name=%q addr=%q", targetName, addr)
	}

	spawnReq := ghost.SpawnGhostRequest{
		TargetName: suffix,
		AdminAddr:  addr,
	}
	spawnOut, spawnErr := root.Admin.SpawnGhost(spawnReq)
	if spawnErr != nil {
		return fmt.Errorf("provision ghost target failed: %w", spawnErr)
	}
	ghostID = spawnOut.GhostID
	addr = spawnOut.AdminAddr

	cfg := ghostTargetConfig{Name: targetName, Addr: addr, GhostID: ghostID}
	a.ghostCfg.Targets = append(a.ghostCfg.Targets, cfg)
	a.targets = append(a.targets, GhostTarget{Name: targetName, Admin: NewRemoteGhostAdmin(addr)})
	if a.activeTarget < 0 {
		a.activeTarget = 0
	}
	logs.Infof("provisioned ghost target name=%q ghost_id=%q addr=%q", targetName, ghostID, addr)
	return a.saveConfigs()
}

// removeGhostTarget deletes one target from runtime and persisted config.
func (a *App) removeGhostTarget() error {
	if len(a.targets) == 0 {
		return errors.New("no targets to remove")
	}
	a.listTargets()
	choice, err := a.promptInt("Remove target", 1, len(a.targets), true, true)
	if err != nil {
		return err
	}
	idx := choice - 1
	name := a.targets[idx].Name
	admin := a.targets[idx].Admin
	a.targets = append(a.targets[:idx], a.targets[idx+1:]...)
	a.ghostCfg.Targets = append(a.ghostCfg.Targets[:idx], a.ghostCfg.Targets[idx+1:]...)
	_ = admin.Close()
	if len(a.targets) == 0 {
		a.activeTarget = -1
	} else if a.activeTarget >= len(a.targets) {
		a.activeTarget = len(a.targets) - 1
	}
	logs.Infof("removed target name=%q", name)
	return a.saveConfigs()
}

// resetToDefaultConfig removes stale targets and restores baseline files.
func (a *App) resetToDefaultConfig() error {
	confirm, err := a.promptLine("Type RESET to confirm")
	if err != nil {
		return err
	}
	if strings.TrimSpace(confirm) != "RESET" {
		return errors.New("reset cancelled")
	}
	a.ghostCfg = ghostConfigFile{
		ClearScreenAfterCommand: false,
		Targets: []ghostTargetConfig{
			{Name: "local-ghost", Addr: "127.0.0.1:7010", GhostID: "ghost.local"},
		},
	}
	a.mirageCfg = mirageConfigFile{
		Targets: []mirageTargetConfig{
			{Name: "local-mirage", Addr: "127.0.0.1:7020", MirageID: "mirage.local"},
		},
		LocalGhostID:        "ghost.local",
		LocalGhostAdminAddr: "127.0.0.1:7010",
	}
	a.closeMirageTargets()
	a.targets = []GhostTarget{{Name: "local-ghost", Admin: NewRemoteGhostAdmin("127.0.0.1:7010")}}
	a.mirageTargets = []MirageTarget{{Name: "local-mirage", Admin: NewRemoteMirageAdmin("127.0.0.1:7020")}}
	a.activeTarget = 0
	a.activeMirage = 0
	a.clearScreen = false
	return a.saveConfigs()
}

func (a *App) selectActiveTarget() error {
	if len(a.targets) == 0 {
		return errors.New("no targets available")
	}
	a.listTargets()
	choice, err := a.promptInt("Select target", 1, len(a.targets), true, true)
	if err != nil {
		return err
	}
	a.activeTarget = choice - 1
	logs.Infof("active target set name=%q", a.targets[a.activeTarget].Name)
	return nil
}

func (a *App) showActiveTargetSummary() {
	target, ok := a.active()
	if !ok {
		fmt.Println("No active target. Add/select one first.")
		return
	}
	a.showGhostTargetSummary(target)
}

func (a *App) showGhostTargetSummary(target GhostTarget) {
	status, err := target.Admin.Status()
	if err != nil {
		fmt.Printf("Status error: %v\n", err)
		return
	}
	seedsList, err := target.Admin.ListSeeds()
	if err != nil {
		fmt.Printf("Seed list error: %v\n", err)
		return
	}

	fmt.Println()
	fmt.Printf("Active Target: %s\n", target.Name)
	fmt.Printf("  addr:     %s\n", target.Admin.Address())
	fmt.Printf("  ghost_id: %s\n", status.GhostID)
	fmt.Printf("  phase:    %s\n", status.Phase)
	fmt.Printf("  seeds:    %d\n", status.SeedCount)
	fmt.Println("  seed ids:")
	for _, seed := range seedsList {
		fmt.Printf("    - %s\n", seed.ID)
	}
}

func (a *App) runGhostAdminConsole() error {
	target, ok := a.active()
	if !ok {
		return errors.New("no active target")
	}
	return a.runGhostAdminConsoleForTarget(target)
}

// runGhostAdminConsoleForTarget drives one admin session for the selected Ghost target.
func (a *App) runGhostAdminConsoleForTarget(target GhostTarget) error {
	for {
		fmt.Println()
		fmt.Printf("Ghost Admin Console (%s @ %s)\n", target.Name, target.Admin.Address())
		fmt.Println("  1) Show status")
		fmt.Println("  2) List seeds and operations")
		fmt.Println("  3) Execute seed command")
		fmt.Println("  4) Lookup execution by command_id")
		fmt.Println("  5) Show recent events")
		fmt.Println("  6) Protocol/message verification view")
		fmt.Println("  7) Back")

		choice, err := a.promptInt("Choose", 1, 7, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			a.showGhostTargetSummary(target)
		case 2:
			if err := a.listSeedOperations(target); err != nil {
				logs.Errf("list seed operations failed: %v", err)
			}
		case 3:
			if err := a.executeSeedCommand(target); err != nil {
				logs.Errf("execute command failed: %v", err)
			}
		case 4:
			if err := a.lookupExecution(target); err != nil {
				logs.Errf("lookup execution failed: %v", err)
			}
		case 5:
			if err := a.showRecentEvents(target); err != nil {
				logs.Errf("show events failed: %v", err)
			}
		case 6:
			if err := a.showVerification(target); err != nil {
				logs.Errf("show verification failed: %v", err)
			}
		case 7:
			return nil
		}
	}
}

func (a *App) listSeedOperations(target GhostTarget) error {
	seedList, err := target.Admin.ListSeeds()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Seed Operations")
	for _, seedMeta := range seedList {
		fmt.Printf("  %s\n", seedMeta.ID)
		specs := operationsForSeed(seedMeta.ID)
		if len(specs) == 0 {
			fmt.Println("    - (operations unknown)")
			continue
		}
		for _, spec := range specs {
			idempotent := "no"
			if spec.Idempotent {
				idempotent = "yes"
			}
			fmt.Printf("    - %s (idempotent=%s)\n", spec.Name, idempotent)
		}
	}
	return nil
}

func (a *App) executeSeedCommand(target GhostTarget) error {
	if err := a.listSeedOperations(target); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Execute Command Input")
	intentIDRaw, err := a.promptLine("intent_id (blank = auto)")
	if err != nil {
		return err
	}
	seedRaw, err := a.promptLine("seed_selector (seed.flow or seed.mongod)")
	if err != nil {
		return err
	}
	operationRaw, err := a.promptLine("operation")
	if err != nil {
		return err
	}
	argsRaw, err := a.promptLine("args key=value,key=value (blank = none)")
	if err != nil {
		return err
	}

	cmd := GhostAdminCommand{
		IntentID:     strings.TrimSpace(intentIDRaw),
		SeedSelector: strings.TrimSpace(seedRaw),
		Operation:    strings.TrimSpace(operationRaw),
		Args:         parseArgsCSV(argsRaw),
	}

	execState, event, err := target.Admin.Execute(cmd)
	if err != nil {
		return err
	}

	fmt.Println()
	fmt.Println("Execution Result")
	fmt.Printf("  command_id:    %s\n", execState.CommandID)
	fmt.Printf("  execution_id:  %s\n", execState.ExecutionID)
	fmt.Printf("  outcome:       %s\n", event.Outcome)
	fmt.Printf("  seed_status:   %s\n", execState.SeedResult.Status)
	fmt.Printf("  seed_exitcode: %d\n", execState.SeedResult.ExitCode)
	if len(execState.SeedResult.Stdout) > 0 {
		fmt.Printf("  stdout:\n%s", indentLines(string(execState.SeedResult.Stdout), "    "))
	}
	if len(execState.SeedResult.Stderr) > 0 {
		fmt.Printf("  stderr:\n%s", indentLines(string(execState.SeedResult.Stderr), "    "))
	}
	return nil
}

func (a *App) lookupExecution(target GhostTarget) error {
	commandIDRaw, err := a.promptLine("command_id")
	if err != nil {
		return err
	}
	commandID := strings.TrimSpace(commandIDRaw)
	if commandID == "" {
		return errors.New("command_id required")
	}

	execState, ok, err := target.Admin.ExecutionByCommandID(commandID)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Printf("No execution found for command_id=%s\n", commandID)
		return nil
	}

	fmt.Println()
	fmt.Println("Execution State")
	fmt.Printf("  command_id:    %s\n", execState.CommandID)
	fmt.Printf("  execution_id:  %s\n", execState.ExecutionID)
	fmt.Printf("  phase:         %s\n", execState.Phase)
	fmt.Printf("  seed_selector: %s\n", execState.SeedSelector)
	fmt.Printf("  operation:     %s\n", execState.Operation)
	fmt.Printf("  outcome:       %s\n", execState.Outcome)
	fmt.Printf("  event_id:      %s\n", execState.Event.EventID)
	return nil
}

func (a *App) showRecentEvents(target GhostTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}

	events, err := target.Admin.RecentEvents(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Recent Events")
	if len(events) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for _, evt := range events {
		ts := time.UnixMilli(int64(evt.TimestampMS)).Format(time.RFC3339)
		fmt.Printf(
			"  event_id=%s command_id=%s seed_id=%s outcome=%s ts=%s\n",
			evt.EventID,
			evt.CommandID,
			evt.SeedID,
			evt.Outcome,
			ts,
		)
	}
	return nil
}

func (a *App) showVerification(target GhostTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}
	records, err := target.Admin.Verification(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Protocol/Message Verification")
	if len(records) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	fmt.Printf("  records=%d\n", len(records))
	fmt.Println()
	fmt.Println("  #  command_id                 execution_id               outcome  seed_status  exit")
	fmt.Println("  -- -------------------------- -------------------------- -------- ------------ ----")
	for i, rec := range records {
		fmt.Printf(
			"  %-2d %-26s %-26s %-8s %-12s %-4d\n",
			i+1,
			truncateRight(rec.CommandID, 26),
			truncateRight(rec.ExecutionID, 26),
			truncateRight(rec.Outcome, 8),
			truncateRight(rec.SeedStatus, 12),
			rec.ExitCode,
		)
	}
	fmt.Println()
	for i, rec := range records {
		ts := time.UnixMilli(int64(rec.TimestampMS)).Format(time.RFC3339)
		fmt.Printf("  [%d] request=%s trace=%s\n", i+1, rec.RequestID, rec.TraceID)
		fmt.Printf("      message: id=%d type=%d -> event_type=%d\n", rec.CommandMessageID, rec.CommandMessageType, rec.EventMessageType)
		fmt.Printf("      ids: command=%s execution=%s event=%s\n", rec.CommandID, rec.ExecutionID, rec.EventID)
		fmt.Printf("      target: ghost=%s seed=%s operation=%s\n", rec.GhostID, rec.SeedID, rec.Operation)
		fmt.Printf("      result: outcome=%s seed_status=%s exit=%d status=%s ts=%s\n", rec.Outcome, rec.SeedStatus, rec.ExitCode, rec.Status, ts)
	}
	return nil
}

func (a *App) submitMirageIssue(target MirageTarget) error {
	fmt.Println()
	fmt.Println("Submit Issue Input")
	intentID, err := a.promptLine("intent_id")
	if err != nil {
		return err
	}
	actor, err := a.promptLine("actor")
	if err != nil {
		return err
	}
	targetScope, err := a.promptLine("target_scope (example: ghost:ghost.local)")
	if err != nil {
		return err
	}
	objective, err := a.promptLine("objective")
	if err != nil {
		return err
	}
	ghostID, err := a.promptLine("command ghost_id")
	if err != nil {
		return err
	}
	seedSelector, err := a.promptLine("command seed_selector")
	if err != nil {
		return err
	}
	operation, err := a.promptLine("command operation")
	if err != nil {
		return err
	}
	argsRaw, err := a.promptLine("command args key=value,key=value (blank = none)")
	if err != nil {
		return err
	}
	blockingRaw, err := a.promptLine("command blocking (true/false, default false)")
	if err != nil {
		return err
	}
	blocking := strings.EqualFold(strings.TrimSpace(blockingRaw), "true")
	req := MirageIssueRequest{
		IntentID:    strings.TrimSpace(intentID),
		Actor:       strings.TrimSpace(actor),
		TargetScope: strings.TrimSpace(targetScope),
		Objective:   strings.TrimSpace(objective),
		CommandPlan: []MirageIssueCommand{
			{
				GhostID:      strings.TrimSpace(ghostID),
				SeedSelector: strings.TrimSpace(seedSelector),
				Operation:    strings.TrimSpace(operation),
				Args:         parseArgsCSV(argsRaw),
				Blocking:     blocking,
			},
		},
	}
	if err := target.Admin.SubmitIssue(req); err != nil {
		return err
	}
	fmt.Printf("Issue submitted intent_id=%s\n", req.IntentID)
	return nil
}

func (a *App) listMirageIntents(target MirageTarget) error {
	intents, err := target.Admin.ListIntents()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Mirage Intents")
	if len(intents) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range intents {
		fmt.Printf("  [%d] %s\n", i+1, intents[i])
	}
	return nil
}

func (a *App) reconcileMirageIntent(target MirageTarget) error {
	intentID, err := a.promptLine("intent_id")
	if err != nil {
		return err
	}
	report, err := target.Admin.ReconcileIntent(strings.TrimSpace(intentID))
	if err != nil {
		return err
	}
	printMirageReport("Reconcile Result", report)
	return nil
}

func (a *App) reconcileAllMirageIntents(target MirageTarget) error {
	reports, err := target.Admin.ReconcileAll()
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Printf("Reconcile All Reports: %d\n", len(reports))
	for i := range reports {
		printMirageReport(fmt.Sprintf("Report %d", i+1), reports[i])
	}
	return nil
}

func (a *App) snapshotMirageIntent(target MirageTarget) error {
	intentID, err := a.promptLine("intent_id")
	if err != nil {
		return err
	}
	snapshot, found, err := target.Admin.SnapshotIntent(strings.TrimSpace(intentID))
	if err != nil {
		return err
	}
	fmt.Println()
	if !found {
		fmt.Printf("Intent %q not found\n", strings.TrimSpace(intentID))
		return nil
	}
	fmt.Printf("Intent Snapshot: %s\n", snapshot.Desired.Issue.IntentID)
	fmt.Printf("  pending_commands: %d\n", snapshot.PendingCount)
	fmt.Printf("  has_observed:     %v\n", snapshot.HasObserved)
	fmt.Printf("  desired_commands: %d\n", len(snapshot.Desired.Commands))
	if snapshot.HasObserved {
		fmt.Printf("  observed_events:  %d\n", len(snapshot.Observed.Events))
		fmt.Printf("  observed_reports: %d\n", len(snapshot.Observed.Reports))
	}
	return nil
}

func (a *App) showMirageReports(target MirageTarget) error {
	limit, err := a.promptOptionalLimit()
	if err != nil {
		return err
	}
	reports, err := target.Admin.RecentReports(limit)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Recent Mirage Reports")
	if len(reports) == 0 {
		fmt.Println("  (none)")
		return nil
	}
	for i := range reports {
		printMirageReport(fmt.Sprintf("Report %d", i+1), reports[i])
	}
	return nil
}

func (a *App) spawnMirageLocalGhost(target MirageTarget) error {
	name, err := a.promptLine("target_name suffix")
	if err != nil {
		return err
	}
	adminAddrRaw, err := a.promptLine("admin addr (host:port or port)")
	if err != nil {
		return err
	}
	adminAddr := strings.TrimSpace(adminAddrRaw)
	if adminAddr == "" {
		return errors.New("admin addr required")
	}
	if !strings.Contains(adminAddr, ":") {
		adminAddr = "127.0.0.1:" + adminAddr
	}
	req := mirage.SpawnGhostRequest{
		TargetName: normalizeSuffix(name),
		AdminAddr:  adminAddr,
	}
	out, err := target.Admin.SpawnLocalGhost(req)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Spawn Local Ghost Result")
	fmt.Printf("  target_name: %s\n", out.TargetName)
	fmt.Printf("  ghost_id:    %s\n", out.GhostID)
	fmt.Printf("  admin_addr:  %s\n", out.AdminAddr)
	return nil
}

func printMirageReport(header string, report session.Report) {
	ts := ""
	if report.TimestampMS > 0 {
		ts = time.UnixMilli(int64(report.TimestampMS)).Format(time.RFC3339)
	}
	fmt.Println()
	fmt.Println(header)
	fmt.Printf("  intent_id:         %s\n", report.IntentID)
	fmt.Printf("  phase:             %s\n", report.Phase)
	fmt.Printf("  completion_state:  %s\n", report.CompletionState)
	fmt.Printf("  summary:           %s\n", report.Summary)
	fmt.Printf("  command_id:        %s\n", report.CommandID)
	fmt.Printf("  execution_id:      %s\n", report.ExecutionID)
	fmt.Printf("  event_id:          %s\n", report.EventID)
	fmt.Printf("  outcome:           %s\n", report.Outcome)
	if ts != "" {
		fmt.Printf("  timestamp:         %s\n", ts)
	}
}

func (a *App) promptOptionalLimit() (int, error) {
	limitRaw, err := a.promptLine("limit (default 20)")
	if err != nil {
		return 0, err
	}
	limit := 20
	if strings.TrimSpace(limitRaw) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(limitRaw))
		if parseErr != nil || parsed <= 0 {
			return 0, errors.New("limit must be a positive integer")
		}
		limit = parsed
	}
	return limit, nil
}

func (a *App) active() (GhostTarget, bool) {
	if a.activeTarget < 0 || a.activeTarget >= len(a.targets) {
		return GhostTarget{}, false
	}
	return a.targets[a.activeTarget], true
}

func (a *App) promptLine(label string) (string, error) {
	fmt.Printf("%s: ", label)
	line, err := a.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimRight(line, "\r\n"), nil
}

func (a *App) promptInt(label string, min int, max int, allowBack bool, allowExit bool) (int, error) {
	for {
		rangePrompt := fmt.Sprintf("%s [%d-%d", label, min, max)
		if allowBack {
			rangePrompt += "|back"
		}
		if allowExit {
			rangePrompt += "|exit"
		}
		rangePrompt += "]"
		line, err := a.promptLine(rangePrompt)
		if err != nil {
			return 0, err
		}
		trimmed := strings.ToLower(strings.TrimSpace(line))
		if allowBack && trimmed == "back" {
			return 0, ErrNavigateBack
		}
		if allowExit && trimmed == "exit" {
			return 0, ErrNavigateExit
		}
		v, err := strconv.Atoi(trimmed)
		if err != nil || v < min || v > max {
			fmt.Println("Invalid selection.")
			continue
		}
		return v, nil
	}
}

func NewRemoteGhostAdmin(addr string) *RemoteGhostAdmin {
	return &RemoteGhostAdmin{addr: strings.TrimSpace(addr)}
}

func NewRemoteMirageAdmin(addr string) *RemoteMirageAdmin {
	return &RemoteMirageAdmin{addr: strings.TrimSpace(addr)}
}

func (c *RemoteGhostAdmin) GhostID() string {
	status, err := c.Status()
	if err != nil {
		return ""
	}
	return status.GhostID
}

func (c *RemoteGhostAdmin) Address() string {
	return c.addr
}

func (c *RemoteGhostAdmin) Status() (ghost.LifecycleStatus, error) {
	var status ghost.LifecycleStatus
	if err := c.call(controlRequest{Action: "status"}, &status); err != nil {
		return ghost.LifecycleStatus{}, err
	}
	return status, nil
}

func (c *RemoteGhostAdmin) ListSeeds() ([]seeds.SeedMetadata, error) {
	var list []seeds.SeedMetadata
	if err := c.call(controlRequest{Action: "list_seeds"}, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (c *RemoteGhostAdmin) Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error) {
	var out executionResponse
	if err := c.call(controlRequest{Action: "execute", Command: command}, &out); err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}
	return out.Execution, out.Event, nil
}

func (c *RemoteGhostAdmin) ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error) {
	var out executionLookupResponse
	req := controlRequest{
		Action:    "execution_by_command_id",
		CommandID: strings.TrimSpace(commandID),
	}
	if err := c.call(req, &out); err != nil {
		return ghost.ExecutionState{}, false, err
	}
	return out.Execution, out.Found, nil
}

func (c *RemoteGhostAdmin) RecentEvents(limit int) ([]ghost.EventEnv, error) {
	var out []ghost.EventEnv
	if err := c.call(controlRequest{Action: "recent_events", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteGhostAdmin) Verification(limit int) ([]ghost.VerificationRecord, error) {
	var out []ghost.VerificationRecord
	if err := c.call(controlRequest{Action: "verification", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SpawnGhost asks a connected root Ghost to provision a child Ghost node.
func (c *RemoteGhostAdmin) SpawnGhost(req ghost.SpawnGhostRequest) (ghost.SpawnGhostResult, error) {
	var out ghost.SpawnGhostResult
	controlReq := controlRequest{
		Action: "spawn_ghost",
		Spawn:  req,
	}
	if err := c.call(controlReq, &out); err != nil {
		return ghost.SpawnGhostResult{}, err
	}
	return out, nil
}

// call sends one admin request to ghostctl and decodes the response payload.
func (c *RemoteGhostAdmin) call(req controlRequest, out any) error {
	if err := c.ensureConn(); err != nil {
		return err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.conn.Write(payload); err != nil {
		c.resetConn()
		return err
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		c.resetConn()
		return err
	}
	var resp controlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out == nil {
		return nil
	}
	if len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

func (c *RemoteGhostAdmin) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *RemoteGhostAdmin) resetConn() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.r = nil
}

// Close terminates the persistent admin connection for this target.
func (c *RemoteGhostAdmin) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}

func (c *RemoteMirageAdmin) Address() string {
	return c.addr
}

func (c *RemoteMirageAdmin) Status() (mirage.LifecycleStatus, error) {
	var out mirage.LifecycleStatus
	if err := c.call(mirageControlRequest{Action: "status"}, &out); err != nil {
		return mirage.LifecycleStatus{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) SubmitIssue(issue MirageIssueRequest) error {
	return c.call(mirageControlRequest{Action: "submit_issue", Issue: issue}, nil)
}

func (c *RemoteMirageAdmin) ReconcileIntent(intentID string) (session.Report, error) {
	var out session.Report
	req := mirageControlRequest{
		Action:   "reconcile_intent",
		IntentID: strings.TrimSpace(intentID),
	}
	if err := c.call(req, &out); err != nil {
		return session.Report{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) ReconcileAll() ([]session.Report, error) {
	var out mirageReconcileAllResponse
	if err := c.call(mirageControlRequest{Action: "reconcile_all"}, &out); err != nil {
		return nil, err
	}
	return out.Reports, nil
}

func (c *RemoteMirageAdmin) SnapshotIntent(intentID string) (mirage.IntentSnapshot, bool, error) {
	var out mirageSnapshotIntentResponse
	req := mirageControlRequest{
		Action:   "snapshot_intent",
		IntentID: strings.TrimSpace(intentID),
	}
	if err := c.call(req, &out); err != nil {
		return mirage.IntentSnapshot{}, false, err
	}
	return out.Snapshot, out.Found, nil
}

func (c *RemoteMirageAdmin) ListIntents() ([]string, error) {
	var out []string
	if err := c.call(mirageControlRequest{Action: "list_intents"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) RecentReports(limit int) ([]session.Report, error) {
	var out []session.Report
	if err := c.call(mirageControlRequest{Action: "recent_reports", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) SpawnLocalGhost(req mirage.SpawnGhostRequest) (mirage.SpawnGhostResult, error) {
	var out mirage.SpawnGhostResult
	controlReq := mirageControlRequest{
		Action: "spawn_local_ghost",
		Spawn:  req,
	}
	if err := c.call(controlReq, &out); err != nil {
		return mirage.SpawnGhostResult{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) RegisteredGhosts() ([]mirage.RegisteredGhost, error) {
	var out []mirage.RegisteredGhost
	if err := c.call(mirageControlRequest{Action: "registered_ghosts"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) call(req mirageControlRequest, out any) error {
	if err := c.ensureConn(); err != nil {
		return err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.conn.Write(payload); err != nil {
		c.resetConn()
		return err
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		c.resetConn()
		return err
	}
	var resp mirageControlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

func (c *RemoteMirageAdmin) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *RemoteMirageAdmin) resetConn() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.r = nil
}

func (c *RemoteMirageAdmin) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}

func (a *App) closeTargets() {
	for _, t := range a.targets {
		_ = t.Admin.Close()
	}
}

func (a *App) closeMirageTargets() {
	for _, t := range a.mirageTargets {
		_ = t.Admin.Close()
	}
}

func parseArgsCSV(in string) map[string]string {
	raw := strings.TrimSpace(in)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make(map[string]string)
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		key, value, ok := strings.Cut(part, "=")
		if !ok {
			continue
		}
		k := strings.TrimSpace(key)
		if k == "" {
			continue
		}
		out[k] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func indentLines(in string, prefix string) string {
	lines := strings.Split(strings.TrimRight(in, "\n"), "\n")
	if len(lines) == 0 {
		return ""
	}
	var b strings.Builder
	for _, line := range lines {
		b.WriteString(prefix)
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

func operationsForSeed(seedID string) []seeds.OperationSpec {
	switch strings.TrimSpace(seedID) {
	case "seed.flow":
		s := seedflow.NewSeed()
		return sortedOps(s.Operations())
	case "seed.mongod":
		s := seedmongod.NewSeed()
		return sortedOps(s.Operations())
	default:
		return nil
	}
}

func sortedOps(in []seeds.OperationSpec) []seeds.OperationSpec {
	out := make([]seeds.OperationSpec, len(in))
	copy(out, in)
	sort.Slice(out, func(i int, j int) bool {
		return out[i].Name < out[j].Name
	})
	return out
}

func truncateRight(in string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(in) <= max {
		return in
	}
	if max <= 3 {
		return in[:max]
	}
	return in[:max-3] + "..."
}

func normalizeSuffix(in string) string {
	raw := strings.ToLower(strings.TrimSpace(in))
	if raw == "" {
		return "node"
	}
	var b strings.Builder
	lastDot := false
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			b.WriteByte(c)
			lastDot = false
			continue
		}
		if !lastDot {
			b.WriteByte('.')
			lastDot = true
		}
	}
	out := strings.Trim(b.String(), ".")
	if out == "" {
		return "node"
	}
	return out
}

func inferGhostIDFromTargetName(name string) string {
	n := strings.TrimSpace(name)
	if n == "" {
		return "ghost.node"
	}
	if n == "local-ghost" {
		return "ghost.local"
	}
	if strings.HasPrefix(n, "local-ghost.") {
		suffix := strings.TrimPrefix(n, "local-ghost.")
		return "ghost.local." + normalizeSuffix(suffix)
	}
	return "ghost." + normalizeSuffix(n)
}

func normalizeTargetAddr(rootAddr string, requested string) (string, error) {
	req := strings.TrimSpace(requested)
	if req == "" {
		return "", errors.New("address required")
	}
	rootHost, _, rootErr := net.SplitHostPort(strings.TrimSpace(rootAddr))
	if rootErr != nil {
		rootHost = "127.0.0.1"
	}
	if strings.Contains(req, ":") {
		host, port, err := net.SplitHostPort(req)
		if err != nil {
			return "", fmt.Errorf("invalid address %q", req)
		}
		if strings.TrimSpace(host) == "" {
			host = rootHost
		}
		if strings.TrimSpace(port) == "" {
			return "", fmt.Errorf("invalid address %q", req)
		}
		return net.JoinHostPort(host, port), nil
	}
	if _, err := strconv.Atoi(req); err != nil {
		return "", fmt.Errorf("invalid port %q", req)
	}
	return net.JoinHostPort(rootHost, req), nil
}

func (a *App) targetExists(name string, addr string) bool {
	for _, t := range a.ghostCfg.Targets {
		if strings.EqualFold(strings.TrimSpace(t.Name), strings.TrimSpace(name)) {
			return true
		}
		if strings.EqualFold(strings.TrimSpace(t.Addr), strings.TrimSpace(addr)) {
			return true
		}
	}
	return false
}

func (a *App) clearIfEnabled() {
	if !a.clearScreen {
		return
	}
	fmt.Print("\033[H\033[2J")
}

// ensureFile creates a missing file and parent directory for config bootstrapping.
func ensureFile(path string) error {
	if _, err := os.Stat(path); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}
