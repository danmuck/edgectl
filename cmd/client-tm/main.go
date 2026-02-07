package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/logging"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

type GhostAdminCommand struct {
	IntentID     string
	SeedSelector string
	Operation    string
	Args         map[string]string
}

type GhostAdmin interface {
	GhostID() string
	Status() (ghost.LifecycleStatus, error)
	ListSeeds() ([]seeds.SeedMetadata, error)
	Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error)
	ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error)
	RecentEvents(limit int) ([]ghost.EventEnv, error)
}

type LocalGhostAdmin struct {
	id      string
	server  *ghost.Server
	nextID  uint64
	mu      sync.Mutex
	events  []ghost.EventEnv
	seedSet map[string]struct{}
}

type GhostTarget struct {
	Name  string
	Admin GhostAdmin
}

type App struct {
	reader       *bufio.Reader
	targets      []GhostTarget
	activeTarget int
}

func main() {
	logging.ConfigureRuntime()
	app := NewApp()
	if err := app.Run(); err != nil {
		logs.Errf("client-tm: %v", err)
		os.Exit(1)
	}
}

func NewApp() *App {
	return &App{
		reader:       bufio.NewReader(os.Stdin),
		targets:      make([]GhostTarget, 0),
		activeTarget: -1,
	}
}

func (a *App) Run() error {
	logs.Infof("client-tm started")
	for {
		a.printMainMenu()
		choice, err := a.promptInt("Choose", 1, 6)
		if err != nil {
			return err
		}
		switch choice {
		case 1:
			a.listTargets()
		case 2:
			if err := a.addDemoGhostTarget(); err != nil {
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
			logs.Infof("client-tm exiting")
			return nil
		}
	}
}

func (a *App) printMainMenu() {
	fmt.Println()
	fmt.Println("Client TM")
	fmt.Println("  1) List ghost targets")
	fmt.Println("  2) Add demo ghost target (flow + mongod)")
	fmt.Println("  3) Select active ghost target")
	fmt.Println("  4) Show active target summary")
	fmt.Println("  5) Ghost admin console")
	fmt.Println("  6) Exit")
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
			fmt.Printf("  %s [%d] %s (status err: %v)\n", marker, i+1, target.Name, err)
			continue
		}
		fmt.Printf(
			"  %s [%d] %s  ghost_id=%s phase=%s seeds=%d\n",
			marker,
			i+1,
			target.Name,
			status.GhostID,
			status.Phase,
			status.SeedCount,
		)
	}
}

func (a *App) addDemoGhostTarget() error {
	nameRaw, err := a.promptLine("Target name (blank = auto)")
	if err != nil {
		return err
	}
	name := strings.TrimSpace(nameRaw)
	if name == "" {
		name = fmt.Sprintf("ghost-demo-%d", len(a.targets)+1)
	}

	ghostIDRaw, err := a.promptLine("Ghost ID (blank = same as target name)")
	if err != nil {
		return err
	}
	ghostID := strings.TrimSpace(ghostIDRaw)
	if ghostID == "" {
		ghostID = name
	}

	admin, err := NewLocalGhostAdmin(ghostID)
	if err != nil {
		return err
	}
	target := GhostTarget{Name: name, Admin: admin}
	a.targets = append(a.targets, target)
	if a.activeTarget < 0 {
		a.activeTarget = 0
	}
	logs.Infof("added ghost target name=%q ghost_id=%q", target.Name, admin.GhostID())
	return nil
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
		fmt.Printf("Ghost Admin Console (%s)\n", target.Name)
		fmt.Println("  1) Show status")
		fmt.Println("  2) List seeds and operations")
		fmt.Println("  3) Execute seed command")
		fmt.Println("  4) Lookup execution by command_id")
		fmt.Println("  5) Show recent events")
		fmt.Println("  6) Back")

		choice, err := a.promptInt("Choose", 1, 6)
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
	status, err := target.Admin.Status()
	if err != nil {
		return err
	}
	intentIDRaw, err := a.promptLine("intent_id (blank = auto)")
	if err != nil {
		return err
	}
	intentID := strings.TrimSpace(intentIDRaw)
	if intentID == "" {
		intentID = fmt.Sprintf("intent.demo.%d", time.Now().Unix())
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
		IntentID:     intentID,
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
	fmt.Printf("  ghost_id:      %s\n", status.GhostID)
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
	limitRaw, err := a.promptLine("limit (default 20)")
	if err != nil {
		return err
	}
	limit := 20
	if strings.TrimSpace(limitRaw) != "" {
		parsed, parseErr := strconv.Atoi(strings.TrimSpace(limitRaw))
		if parseErr != nil || parsed <= 0 {
			return errors.New("limit must be a positive integer")
		}
		limit = parsed
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

func NewLocalGhostAdmin(ghostID string) (*LocalGhostAdmin, error) {
	id := strings.TrimSpace(ghostID)
	if id == "" {
		return nil, errors.New("ghost_id required")
	}

	server := ghost.NewServer()
	if err := server.Appear(ghost.GhostConfig{GhostID: id}); err != nil {
		return nil, err
	}
	reg := seeds.NewRegistry()
	if err := reg.Register(seeds.NewFlowSeed()); err != nil {
		return nil, err
	}
	if err := reg.Register(seeds.NewMongodSeed()); err != nil {
		return nil, err
	}
	if err := server.Seed(reg); err != nil {
		return nil, err
	}
	if err := server.Radiate(); err != nil {
		return nil, err
	}

	seedSet := map[string]struct{}{}
	for _, meta := range reg.ListMetadata() {
		seedSet[meta.ID] = struct{}{}
	}
	return &LocalGhostAdmin{
		id:      id,
		server:  server,
		events:  make([]ghost.EventEnv, 0),
		seedSet: seedSet,
	}, nil
}

func (a *LocalGhostAdmin) GhostID() string {
	return a.id
}

func (a *LocalGhostAdmin) Status() (ghost.LifecycleStatus, error) {
	return a.server.Status(), nil
}

func (a *LocalGhostAdmin) ListSeeds() ([]seeds.SeedMetadata, error) {
	return a.server.SeedMetadata(), nil
}

func (a *LocalGhostAdmin) Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error) {
	seedID := strings.TrimSpace(command.SeedSelector)
	if _, ok := a.seedSet[seedID]; !ok {
		return ghost.ExecutionState{}, ghost.EventEnv{}, fmt.Errorf("unsupported seed_selector: %s", seedID)
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	a.nextID++
	messageID := a.nextID
	commandID := fmt.Sprintf("cmd.%s.%d", a.id, messageID)
	cmd := ghost.CommandEnv{
		MessageID:    messageID,
		CommandID:    commandID,
		IntentID:     strings.TrimSpace(command.IntentID),
		GhostID:      a.id,
		SeedSelector: seedID,
		Operation:    strings.TrimSpace(command.Operation),
		Args:         cloneArgs(command.Args),
	}
	if cmd.IntentID == "" {
		cmd.IntentID = fmt.Sprintf("intent.%s.%d", a.id, messageID)
	}

	event, err := a.server.HandleCommandAndExecute(cmd)
	if err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}
	execState, ok := a.server.ExecutionByCommandID(commandID)
	if !ok {
		return ghost.ExecutionState{}, ghost.EventEnv{}, fmt.Errorf("missing execution state for command_id=%s", commandID)
	}
	a.events = append(a.events, event)
	return execState, event, nil
}

func (a *LocalGhostAdmin) ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error) {
	execState, ok := a.server.ExecutionByCommandID(strings.TrimSpace(commandID))
	return execState, ok, nil
}

func (a *LocalGhostAdmin) RecentEvents(limit int) ([]ghost.EventEnv, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	if len(a.events) == 0 {
		return []ghost.EventEnv{}, nil
	}
	start := 0
	if len(a.events) > limit {
		start = len(a.events) - limit
	}
	out := make([]ghost.EventEnv, len(a.events[start:]))
	copy(out, a.events[start:])
	return out, nil
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

func cloneArgs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
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
