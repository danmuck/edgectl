package main

import (
	"bufio"
	"encoding/json"
	"errors"
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
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

const (
	ghostConfigPath  = "cmd/client-tm/ghost.toml"
	mirageConfigPath = "cmd/client-tm/mirage.toml"
)

// ghostConfigFile persists Ghost targets configured for the client.
type ghostConfigFile struct {
	Targets []ghostTargetConfig `toml:"targets"`
}

// ghostTargetConfig binds a display name to a Ghost admin endpoint.
type ghostTargetConfig struct {
	Name string `toml:"name"`
	Addr string `toml:"addr"`
}

// mirageConfigFile reserves future Mirage target wiring.
type mirageConfigFile struct {
	Targets []mirageTargetConfig `toml:"targets"`
}

type mirageTargetConfig struct {
	Name string `toml:"name"`
	Addr string `toml:"addr"`
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
}

// RemoteGhostAdmin is a TCP client for ghostctl admin control endpoint.
type RemoteGhostAdmin struct {
	addr string
}

// GhostTarget maps a friendly name to a concrete Ghost admin implementation.
type GhostTarget struct {
	Name  string
	Admin GhostAdmin
}

// controlRequest is one line-delimited control request payload.
type controlRequest struct {
	Action    string            `json:"action"`
	Limit     int               `json:"limit,omitempty"`
	CommandID string            `json:"command_id,omitempty"`
	Command   GhostAdminCommand `json:"command,omitempty"`
}

// controlResponse is one line-delimited control response payload.
type controlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
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
}

func main() {
	logging.ConfigureRuntime()
	app := NewApp(ghostConfigPath, mirageConfigPath)
	if err := app.Run(); err != nil {
		logs.Errf("client-tm: %v", err)
		os.Exit(1)
	}
}

func NewApp(ghostCfgPath string, mirageCfgPath string) *App {
	return &App{
		reader:        bufio.NewReader(os.Stdin),
		ghostCfgPath:  ghostCfgPath,
		mirageCfgPath: mirageCfgPath,
		targets:       make([]GhostTarget, 0),
		activeTarget:  -1,
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

	for {
		a.printMainMenu()
		choice, err := a.promptInt("Choose", 1, 7)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			a.listTargets()
		case 2:
			if err := a.addGhostTarget(); err != nil {
				logs.Errf("add target failed: %v", err)
			}
		case 3:
			if err := a.selectActiveTarget(); err != nil {
				logs.Errf("select target failed: %v", err)
			}
		case 4:
			a.showActiveTargetSummary()
		case 5:
			if err := a.runGhostAdminConsole(); err != nil {
				logs.Errf("ghost admin console error: %v", err)
			}
		case 6:
			if err := a.saveConfigs(); err != nil {
				logs.Errf("save failed: %v", err)
			} else {
				logs.Infof("config saved")
			}
		case 7:
			if err := a.saveConfigs(); err != nil {
				logs.Warnf("save on exit failed: %v", err)
			}
			logs.Infof("client-tm exiting")
			return nil
		}
	}
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

	for _, cfg := range a.ghostCfg.Targets {
		name := strings.TrimSpace(cfg.Name)
		addr := strings.TrimSpace(cfg.Addr)
		if name == "" || addr == "" {
			continue
		}
		a.targets = append(a.targets, GhostTarget{
			Name:  name,
			Admin: NewRemoteGhostAdmin(addr),
		})
	}
	if len(a.targets) > 0 {
		a.activeTarget = 0
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
	fmt.Printf("  mirage config: %s (targets=%d, not yet wired)\n", a.mirageCfgPath, len(a.mirageCfg.Targets))
	fmt.Println("  1) List ghost targets")
	fmt.Println("  2) Add ghost target (persist)")
	fmt.Println("  3) Select active ghost target")
	fmt.Println("  4) Show active target summary")
	fmt.Println("  5) Ghost admin console")
	fmt.Println("  6) Save configs")
	fmt.Println("  7) Exit")
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
	nameRaw, err := a.promptLine("Target name")
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

	cfg := ghostTargetConfig{Name: name, Addr: addr}
	a.ghostCfg.Targets = append(a.ghostCfg.Targets, cfg)
	a.targets = append(a.targets, GhostTarget{Name: name, Admin: NewRemoteGhostAdmin(addr)})
	if a.activeTarget < 0 {
		a.activeTarget = 0
	}
	return a.saveConfigs()
}

func (a *App) selectActiveTarget() error {
	if len(a.targets) == 0 {
		return errors.New("no targets available")
	}
	a.listTargets()
	choice, err := a.promptInt("Select target", 1, len(a.targets))
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

		choice, err := a.promptInt("Choose", 1, 7)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			a.showActiveTargetSummary()
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
	for _, rec := range records {
		ts := time.UnixMilli(int64(rec.TimestampMS)).Format(time.RFC3339)
		fmt.Printf(
			"  req=%s trace=%s msg_id=%d cmd_id=%s exec_id=%s evt_id=%s op=%s seed=%s outcome=%s seed_status=%s exit=%d ts=%s\n",
			rec.RequestID,
			rec.TraceID,
			rec.CommandMessageID,
			rec.CommandID,
			rec.ExecutionID,
			rec.EventID,
			rec.Operation,
			rec.SeedID,
			rec.Outcome,
			rec.SeedStatus,
			rec.ExitCode,
			ts,
		)
	}
	return nil
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

func (a *App) promptInt(label string, min int, max int) (int, error) {
	for {
		line, err := a.promptLine(fmt.Sprintf("%s [%d-%d]", label, min, max))
		if err != nil {
			return 0, err
		}
		v, err := strconv.Atoi(strings.TrimSpace(line))
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

// call sends one admin request to ghostctl and decodes the response payload.
func (c *RemoteGhostAdmin) call(req controlRequest, out any) error {
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := conn.Write(payload); err != nil {
		return err
	}

	reader := bufio.NewReader(conn)
	line, err := reader.ReadBytes('\n')
	if err != nil {
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
		s := seeds.NewFlowSeed()
		return sortedOps(s.Operations())
	case "seed.mongod":
		s := seeds.NewMongodSeed()
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
