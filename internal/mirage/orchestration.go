package mirage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

var (
	ErrInvalidIssue        = errors.New("mirage: invalid issue")
	ErrIntentNotFound      = errors.New("mirage: intent not found")
	ErrTargetGhostRequired = errors.New("mirage: target ghost required")
)

const (
	ReportPhaseInProgress = "in_progress"
	ReportPhaseComplete   = "complete"

	CompletionInProgress = "in_progress"
	CompletionSatisfied  = "satisfied"
	CompletionFailed     = "failed"

	OutcomeSuccess = "success"
	OutcomeError   = "error"
)

// IssueCommand is one desired command step inside a complex intent plan.
type IssueCommand struct {
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
	Blocking     bool
}

// Validate enforces command-step fields required for orchestration planning.
func (c IssueCommand) Validate() error {
	if strings.TrimSpace(c.GhostID) == "" {
		return fmt.Errorf("%w: missing ghost_id", ErrInvalidIssue)
	}
	if strings.TrimSpace(c.SeedSelector) == "" {
		return fmt.Errorf("%w: missing seed_selector", ErrInvalidIssue)
	}
	if strings.TrimSpace(c.Operation) == "" {
		return fmt.Errorf("%w: missing operation", ErrInvalidIssue)
	}
	return nil
}

// Stages of a multi-stage issue
type IssueStage struct {
	ID       string
	Commands []IssueCommand
	Barrier  bool
}

// IssueEnv is Mirage desired-state ingress from the user boundary.
type IssueEnv struct {
	IntentID         string
	Actor            string
	TargetScope      string
	Objective        string
	SeedDependencies []string
	TimestampMS      uint64

	Stages []IssueStage `json:"stages,omitempty"`
	// CommandPlan is optional; when present it defines all single-command loops.
	CommandPlan []IssueCommand

	// Legacy single-command fields used when CommandPlan is empty.
	SeedSelector string
	Operation    string
	Args         map[string]string
}

// Validate enforces required issue fields for desired-state ingestion.
func (i IssueEnv) Validate() error {
	if strings.TrimSpace(i.IntentID) == "" {
		return fmt.Errorf("%w: missing intent_id", ErrInvalidIssue)
	}
	if strings.TrimSpace(i.Actor) == "" {
		return fmt.Errorf("%w: missing actor", ErrInvalidIssue)
	}
	if strings.TrimSpace(i.TargetScope) == "" {
		return fmt.Errorf("%w: missing target_scope", ErrInvalidIssue)
	}
	if strings.TrimSpace(i.Objective) == "" {
		return fmt.Errorf("%w: missing objective", ErrInvalidIssue)
	}
	for idx := range i.CommandPlan {
		if err := i.CommandPlan[idx].Validate(); err != nil {
			return fmt.Errorf("%w: command_plan[%d]: %v", ErrInvalidIssue, idx, err)
		}
	}
	return nil
}

// DesiredIntent stores one normalized command plan derived from an issue.
type DesiredIntent struct {
	Issue      IssueEnv
	Commands   []PlannedCommand
	ReceivedAt time.Time
}

// PlannedCommand is one normalized desired command plus orchestration hints.
type PlannedCommand struct {
	Command  session.Command
	Blocking bool
	SeedKey  string
}

// ObservedIntent stores event/report history for one intent reconcile flow.
type ObservedIntent struct {
	Events      []session.Event
	Reports     []session.Report
	ObservedAt  time.Time
	ByCommandID map[string]session.Event
}

// IntentSnapshot is a read-only projection of desired and optional observed state.
type IntentSnapshot struct {
	Desired      DesiredIntent
	Observed     ObservedIntent
	HasObserved  bool
	PendingCount int
}

// CommandExecutor executes a command payload and returns one terminal event.
type CommandExecutor interface {
	ExecuteCommand(ctx context.Context, cmd session.Command) (session.Event, error)
}

// Orchestrator owns Mirage desired/observed stores and reconcile behavior.
type Orchestrator struct {
	mu        sync.RWMutex
	desired   map[string]DesiredIntent
	observed  map[string]*ObservedIntent
	executors map[string]CommandExecutor
	seedLocks map[string]seedLock
	seq       atomic.Uint64
}

type seedLock struct {
	IntentID  string
	CommandID string
}

// OrchestratorSnapshot summarizes desired/observed store sizes.
type OrchestratorSnapshot struct {
	IntentCount   int
	ObservedCount int
}

// NewOrchestrator returns an empty Mirage orchestration loop with in-memory stores.
func NewOrchestrator() *Orchestrator {
	return &Orchestrator{
		desired:   make(map[string]DesiredIntent),
		observed:  make(map[string]*ObservedIntent),
		executors: make(map[string]CommandExecutor),
		seedLocks: make(map[string]seedLock),
	}
}

// Snapshot returns aggregate desired/observed store counters.
func (o *Orchestrator) Snapshot() OrchestratorSnapshot {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return OrchestratorSnapshot{
		IntentCount:   len(o.desired),
		ObservedCount: len(o.observed),
	}
}

// ListIntentIDs returns sorted intent ids currently persisted in desired state.
func (o *Orchestrator) ListIntentIDs() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]string, 0, len(o.desired))
	for intentID := range o.desired {
		out = append(out, intentID)
	}
	sort.Strings(out)
	return out
}

// RegisterExecutor binds one ghost_id to an execution adapter.
func (o *Orchestrator) RegisterExecutor(ghostID string, exec CommandExecutor) error {
	key := strings.TrimSpace(ghostID)
	if key == "" {
		return ErrTargetGhostRequired
	}
	if exec == nil {
		return fmt.Errorf("mirage: nil executor for ghost %q", key)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.executors[key] = exec
	return nil
}

// SubmitIssue validates, normalizes, and persists desired state for one intent.
func (o *Orchestrator) SubmitIssue(issue IssueEnv) error {
	if err := issue.Validate(); err != nil {
		return err
	}
	now := time.Now()
	if issue.TimestampMS == 0 {
		issue.TimestampMS = uint64(now.UnixMilli())
	}
	stages, err := normalizeIssueToStages(issue)
	if err != nil {
		return err
	}
	commands, err := flattenStagesToPlannedCommands(issue.IntentID, stages)
	if err != nil {
		return err
	}
	issue.SeedDependencies = seedDependenciesForCommands(commands)
	o.mu.Lock()
	defer o.mu.Unlock()
	o.desired[issue.IntentID] = DesiredIntent{
		Issue:      issue,
		Commands:   commands,
		ReceivedAt: now,
	}
	delete(o.observed, issue.IntentID)
	return nil
}

func seedDependenciesForCommands(commands []PlannedCommand) []string {
	seen := make(map[string]struct{}, len(commands))
	out := make([]string, 0, len(commands))
	for i := range commands {
		seedID := strings.TrimSpace(commands[i].Command.SeedSelector)
		if seedID == "" {
			continue
		}
		if _, ok := seen[seedID]; ok {
			continue
		}
		seen[seedID] = struct{}{}
		out = append(out, seedID)
	}
	sort.Strings(out)
	return out
}

// SnapshotIntent returns one intent snapshot with explicit desired/observed visibility.
func (o *Orchestrator) SnapshotIntent(intentID string) (IntentSnapshot, bool) {
	key := strings.TrimSpace(intentID)
	o.mu.RLock()
	defer o.mu.RUnlock()

	desired, ok := o.desired[key]
	if !ok {
		return IntentSnapshot{}, false
	}
	out := IntentSnapshot{Desired: desired}

	obs, ok := o.observed[key]
	if !ok {
		out.PendingCount = len(desired.Commands)
		return out, true
	}
	out.HasObserved = true
	out.Observed = cloneObserved(*obs)
	out.PendingCount = pendingCommandCount(desired.Commands, obs.ByCommandID)
	return out, true
}

// ReconcileOnce executes one pending single-command loop and emits one report update.
func (o *Orchestrator) ReconcileOnce(ctx context.Context, intentID string) (session.Report, error) {
	key := strings.TrimSpace(intentID)
	o.mu.Lock()
	desired, ok := o.desired[key]
	obs := o.observed[key]
	if !ok {
		o.mu.Unlock()
		return session.Report{}, fmt.Errorf("%w: %s", ErrIntentNotFound, key)
	}
	next, hasPending := nextPendingCommand(desired.Commands, obs)
	if !hasPending {
		report := latestReportOrSynthesizeComplete(desired, obs)
		o.mu.Unlock()
		return report, nil
	}

	if next.Blocking {
		if lock, held := o.seedLocks[next.SeedKey]; held {
			if lock.IntentID != key || lock.CommandID != next.Command.CommandID {
				report := buildBlockedReport(desired, next, lock)
				observed := ensureObservedLocked(o, key)
				observed.Reports = append(observed.Reports, report)
				observed.ObservedAt = time.Now()
				o.mu.Unlock()
				return report, nil
			}
		} else {
			o.seedLocks[next.SeedKey] = seedLock{IntentID: key, CommandID: next.Command.CommandID}
		}
	}

	exec := o.executors[next.Command.GhostID]
	o.mu.Unlock()
	if exec == nil {
		return session.Report{}, fmt.Errorf("mirage: no executor registered for ghost_id=%q", next.Command.GhostID)
	}

	wireCommand, err := o.dispatchCommandEnvelope(next.Command)
	if err != nil {
		if next.Blocking {
			o.releaseSeedLock(next.SeedKey, key, next.Command.CommandID)
		}
		return session.Report{}, err
	}
	event, err := exec.ExecuteCommand(ctx, wireCommand)
	if err != nil {
		if next.Blocking {
			o.releaseSeedLock(next.SeedKey, key, next.Command.CommandID)
		}
		return session.Report{}, err
	}

	o.mu.Lock()
	observed := ensureObservedLocked(o, key)
	report, ingestedEvent, err := o.ingestEventEnvelopeAndBuildReport(desired, observed, event)
	if err != nil {
		o.mu.Unlock()
		if next.Blocking {
			o.releaseSeedLock(next.SeedKey, key, next.Command.CommandID)
		}
		return session.Report{}, err
	}
	observed.ByCommandID[ingestedEvent.CommandID] = ingestedEvent
	observed.Events = append(observed.Events, ingestedEvent)
	observed.Reports = append(observed.Reports, report)
	observed.ObservedAt = time.Now()
	if next.Blocking {
		delete(o.seedLocks, next.SeedKey)
	}
	o.mu.Unlock()
	return report, nil
}

// IngestObservedEvent maps an external ghost event into intent observed state.
func (o *Orchestrator) IngestObservedEvent(event session.Event) (session.Report, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	intentID, plan, ok := o.findPlannedCommandByCommandID(event.CommandID)
	if !ok {
		return session.Report{}, false, nil
	}
	desired := o.desired[intentID]
	observed := ensureObservedLocked(o, intentID)
	if _, exists := observed.ByCommandID[event.CommandID]; exists {
		if len(observed.Reports) == 0 {
			report := latestReportOrSynthesizeComplete(desired, observed)
			return report, true, nil
		}
		return observed.Reports[len(observed.Reports)-1], true, nil
	}

	report, ingestedEvent, err := o.ingestEventEnvelopeAndBuildReport(desired, observed, event)
	if err != nil {
		return session.Report{}, true, err
	}
	observed.ByCommandID[ingestedEvent.CommandID] = ingestedEvent
	observed.Events = append(observed.Events, ingestedEvent)
	observed.Reports = append(observed.Reports, report)
	observed.ObservedAt = time.Now()
	if plan.Blocking {
		delete(o.seedLocks, plan.SeedKey)
	}
	return report, true, nil
}

// dispatchCommandEnvelope round-trips command payload through protocol framing.
func (o *Orchestrator) dispatchCommandEnvelope(command session.Command) (session.Command, error) {
	payload, err := session.EncodeCommandFrame(o.seq.Add(1), command)
	if err != nil {
		return session.Command{}, err
	}
	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		return session.Command{}, err
	}
	return session.DecodeCommandFrame(fr)
}

// ingestEventEnvelopeAndBuildReport ingests a framed event and emits a framed report.
func (o *Orchestrator) ingestEventEnvelopeAndBuildReport(
	desired DesiredIntent,
	observed *ObservedIntent,
	event session.Event,
) (session.Report, session.Event, error) {
	eventPayload, err := session.EncodeEventFrame(o.seq.Add(1), event)
	if err != nil {
		return session.Report{}, session.Event{}, err
	}
	eventFrame, err := frame.ReadFrame(bytes.NewReader(eventPayload), frame.DefaultLimits())
	if err != nil {
		return session.Report{}, session.Event{}, err
	}
	ingestedEvent, err := session.DecodeEventFrame(eventFrame)
	if err != nil {
		return session.Report{}, session.Event{}, err
	}

	report := buildReportFromObserved(desired, observed, ingestedEvent)
	reportPayload, err := session.EncodeReportFrame(o.seq.Add(1), report)
	if err != nil {
		return session.Report{}, session.Event{}, err
	}
	reportFrame, err := frame.ReadFrame(bytes.NewReader(reportPayload), frame.DefaultLimits())
	if err != nil {
		return session.Report{}, session.Event{}, err
	}
	wireReport, err := session.DecodeReportFrame(reportFrame)
	if err != nil {
		return session.Report{}, session.Event{}, err
	}
	return wireReport, ingestedEvent, nil
}

func normalizeIssueToStages(issue IssueEnv) ([]IssueStage, error) {
	// explicit stages provided
	if len(issue.Stages) > 0 {
		out := make([]IssueStage, 0, len(issue.Stages))
		for i := range issue.Stages {
			stage := issue.Stages[i]
			if strings.TrimSpace(stage.ID) == "" {
				stage.ID = fmt.Sprintf("stage.%d", i+1)
			}
			for j := range stage.Commands {
				if err := stage.Commands[j].Validate(); err != nil {
					return nil, fmt.Errorf(
						"%w: stages[%d].commands[%d]: %v",
						ErrInvalidIssue, i, j, err,
					)
				}
			}
			out = append(out, stage)
		}
		return out, nil
	}

	// legacy path — explicit CommandPlan
	if len(issue.CommandPlan) > 0 {
		return []IssueStage{
			{
				ID:       "legacy.command_plan",
				Barrier:  true,
				Commands: issue.CommandPlan,
			},
		}, nil
	}

	// legacy shorthand — single implicit command
	ghostID := normalizeGhostID(issue.TargetScope)
	if ghostID == "" {
		return nil, fmt.Errorf("%w: target_scope=%q", ErrTargetGhostRequired, issue.TargetScope)
	}

	seedSelector := strings.TrimSpace(issue.SeedSelector)
	if seedSelector == "" {
		seedSelector = "seed.flow"
	}

	operation := strings.TrimSpace(issue.Operation)
	if operation == "" {
		operation = strings.TrimSpace(issue.Objective)
	}
	if operation == "" {
		return nil, fmt.Errorf("%w: missing operation", ErrInvalidIssue)
	}

	cmd := IssueCommand{
		GhostID:      ghostID,
		SeedSelector: seedSelector,
		Operation:    operation,
		Args:         copyArgs(issue.Args),
		Blocking:     false,
	}

	return []IssueStage{
		{
			ID:       "legacy.single_command",
			Barrier:  true,
			Commands: []IssueCommand{cmd},
		},
	}, nil
}

func flattenStagesToPlannedCommands(
	intentID string,
	stages []IssueStage,
) ([]PlannedCommand, error) {
	var out []PlannedCommand
	seq := 0

	for si := range stages {
		stage := stages[si]
		for ci := range stage.Commands {
			seq++
			step := stage.Commands[ci]
			if err := step.Validate(); err != nil {
				return nil, err
			}

			cmd := session.Command{
				CommandID:    fmt.Sprintf("cmd.%s.%d", sanitizeID(intentID), seq),
				IntentID:     intentID,
				GhostID:      strings.TrimSpace(step.GhostID),
				SeedSelector: strings.TrimSpace(step.SeedSelector),
				Operation:    strings.TrimSpace(step.Operation),
				Args:         copyArgs(step.Args),
			}

			out = append(out, PlannedCommand{
				Command:  cmd,
				Blocking: step.Blocking || stage.Barrier,
				SeedKey:  seedLockKey(cmd.GhostID, cmd.SeedSelector),
			})
		}
	}

	return out, nil
}

// buildReportFromObserved converts one observed event into a report update.
func buildReportFromObserved(desired DesiredIntent, observed *ObservedIntent, event session.Event) session.Report {
	completedAfter := len(observed.ByCommandID) + 1
	total := len(desired.Commands)
	phase := ReportPhaseInProgress
	completion := CompletionInProgress
	summary := fmt.Sprintf(
		"intent %s progress %d/%d on %s",
		desired.Issue.IntentID,
		completedAfter,
		total,
		event.GhostID,
	)
	if event.Outcome == OutcomeError {
		phase = ReportPhaseComplete
		completion = CompletionFailed
		summary = fmt.Sprintf("intent %s failed on %s", desired.Issue.IntentID, event.GhostID)
	} else if completedAfter >= total {
		phase = ReportPhaseComplete
		completion = CompletionSatisfied
		summary = fmt.Sprintf("intent %s satisfied on %s", desired.Issue.IntentID, event.GhostID)
	}
	return session.Report{
		IntentID:        desired.Issue.IntentID,
		Phase:           phase,
		Summary:         summary,
		CompletionState: completion,
		CommandID:       event.CommandID,
		EventID:         event.EventID,
		Outcome:         event.Outcome,
		TimestampMS:     uint64(time.Now().UnixMilli()),
	}
}

// latestReportOrSynthesizeComplete returns existing terminal state for settled intents.
func latestReportOrSynthesizeComplete(desired DesiredIntent, obs *ObservedIntent) session.Report {
	if obs != nil && len(obs.Reports) > 0 {
		return obs.Reports[len(obs.Reports)-1]
	}
	return session.Report{
		IntentID:        desired.Issue.IntentID,
		Phase:           ReportPhaseComplete,
		Summary:         fmt.Sprintf("intent %s has no command plan", desired.Issue.IntentID),
		CompletionState: CompletionSatisfied,
		TimestampMS:     uint64(time.Now().UnixMilli()),
	}
}

// nextPendingCommand returns the first command not yet observed for an intent.
func nextPendingCommand(commands []PlannedCommand, observed *ObservedIntent) (PlannedCommand, bool) {
	if observed == nil {
		if len(commands) == 0 {
			return PlannedCommand{}, false
		}
		return commands[0], true
	}
	for i := range commands {
		cmd := commands[i]
		if _, seen := observed.ByCommandID[cmd.Command.CommandID]; !seen {
			return cmd, true
		}
	}
	return PlannedCommand{}, false
}

// pendingCommandCount returns desired commands still missing observed event closure.
func pendingCommandCount(commands []PlannedCommand, byCommandID map[string]session.Event) int {
	if len(commands) == 0 {
		return 0
	}
	out := 0
	for i := range commands {
		if _, ok := byCommandID[commands[i].Command.CommandID]; !ok {
			out++
		}
	}
	return out
}

// cloneObserved returns a defensive copy of observed state maps/slices.
func cloneObserved(in ObservedIntent) ObservedIntent {
	out := ObservedIntent{
		Events:      append([]session.Event{}, in.Events...),
		Reports:     append([]session.Report{}, in.Reports...),
		ObservedAt:  in.ObservedAt,
		ByCommandID: make(map[string]session.Event, len(in.ByCommandID)),
	}
	maps.Copy(out.ByCommandID, in.ByCommandID)
	return out
}

// normalizeGhostID resolves single-ghost target identifiers from target_scope text.
func normalizeGhostID(targetScope string) string {
	out := strings.TrimSpace(targetScope)
	out = strings.TrimPrefix(out, "ghost:")
	return strings.TrimSpace(out)
}

// sanitizeID converts ids into command-safe dot-separated text.
func sanitizeID(in string) string {
	raw := strings.TrimSpace(in)
	raw = strings.ReplaceAll(raw, " ", ".")
	raw = strings.ReplaceAll(raw, "/", ".")
	return raw
}

// copyArgs returns a defensive copy of command args for desired-state snapshots.
func copyArgs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	maps.Copy(out, in)
	return out
}

// seedLockKey scopes blocking serialization to one ghost/seed tuple.
func seedLockKey(ghostID string, seedSelector string) string {
	return strings.TrimSpace(ghostID) + "::" + strings.TrimSpace(seedSelector)
}

// buildBlockedReport emits an explicit report update when a blocking seed lock is held.
func buildBlockedReport(desired DesiredIntent, next PlannedCommand, lock seedLock) session.Report {
	return session.Report{
		IntentID:        desired.Issue.IntentID,
		Phase:           ReportPhaseInProgress,
		Summary:         fmt.Sprintf("intent %s waiting on seed lock %s held by %s/%s", desired.Issue.IntentID, next.SeedKey, lock.IntentID, lock.CommandID),
		CompletionState: CompletionInProgress,
		CommandID:       next.Command.CommandID,
		Outcome:         OutcomeSuccess,
		TimestampMS:     uint64(time.Now().UnixMilli()),
	}
}

// ensureObservedLocked initializes observed-state storage while orchestrator mutex is held.
func ensureObservedLocked(o *Orchestrator, intentID string) *ObservedIntent {
	observed := o.observed[intentID]
	if observed != nil {
		return observed
	}
	observed = &ObservedIntent{ByCommandID: make(map[string]session.Event)}
	o.observed[intentID] = observed
	return observed
}

// findPlannedCommandByCommandID resolves one command id back to its owning intent and plan entry.
func (o *Orchestrator) findPlannedCommandByCommandID(commandID string) (string, PlannedCommand, bool) {
	key := strings.TrimSpace(commandID)
	for intentID, desired := range o.desired {
		for i := range desired.Commands {
			planned := desired.Commands[i]
			if planned.Command.CommandID == key {
				return intentID, planned, true
			}
		}
	}
	return "", PlannedCommand{}, false
}

// releaseSeedLock clears a held lock only when owner matches to avoid stomping newer owners.
func (o *Orchestrator) releaseSeedLock(seedKey, intentID, commandID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	lock, ok := o.seedLocks[seedKey]
	if !ok {
		return
	}
	if lock.IntentID == intentID && lock.CommandID == commandID {
		delete(o.seedLocks, seedKey)
	}
}

// unused: connectedGhostsForSeed returns ghost IDs that are:
//  1. currently connected (per routing table)
//  2. advertising the given seed
//
// This is a pure planning helper used by intent planners.
// It must not mutate state or perform I/O.
func connectedGhostsForSeed(
	routes []GhostRoute,
	services []AvailableService,
	seedID string,
) []string {
	seedID = strings.TrimSpace(seedID)
	if seedID == "" {
		return nil
	}

	// Build set of connected ghosts
	connected := make(map[string]struct{}, len(routes))
	for i := range routes {
		r := routes[i]
		if r.Connected {
			id := strings.TrimSpace(r.GhostID)
			if id != "" {
				connected[id] = struct{}{}
			}
		}
	}

	// Intersect with seed advertisers
	out := make(map[string]struct{})
	for i := range services {
		svc := services[i]
		if strings.TrimSpace(svc.SeedID) != seedID {
			continue
		}
		for _, ghostID := range svc.GhostIDs {
			g := strings.TrimSpace(ghostID)
			if g == "" {
				continue
			}
			if _, ok := connected[g]; ok {
				out[g] = struct{}{}
			}
		}
	}

	// Stable ordering
	result := make([]string, 0, len(out))
	for g := range out {
		result = append(result, g)
	}
	sort.Strings(result)
	return result
}
