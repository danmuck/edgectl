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

	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

var (
	ErrInvalidIssue = errors.New("mirage: invalid issue")
	// Issue names an existing intent_id that a different actor submitted.
	ErrIntentActorMismatch = errors.New("mirage: intent belongs to another actor")
	ErrIntentNotFound      = errors.New("mirage: intent not found")
	ErrTargetGhostRequired = errors.New("mirage: target ghost required")
	// Event reports on a command planned for a different ghost than the reporting session.
	ErrCommandGhostMismatch = errors.New("mirage: command planned for another ghost")
	// Event names a planned command that Mirage has not dispatched; it cannot be controller feedback.
	ErrCommandNotDispatched = errors.New("mirage: command not dispatched")
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
	Stages           []IssueStage
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
	if len(i.Stages) == 0 {
		return fmt.Errorf("%w: at least one stage required", ErrInvalidIssue)
	}
	for si := range i.Stages {
		stage := i.Stages[si]
		if len(stage.Commands) == 0 {
			return fmt.Errorf("%w: stages[%d]: no commands", ErrInvalidIssue, si)
		}
		for ci := range stage.Commands {
			if err := stage.Commands[ci].Validate(); err != nil {
				return fmt.Errorf("%w: stages[%d].commands[%d]: %v", ErrInvalidIssue, si, ci, err)
			}
		}
	}
	return nil
}

// DesiredIntent stores normalized planned commands derived from issue stages.
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
	// Per-command lifecycle state; terminal entries are not dispatched again.
	CommandStates map[string]CommandStatus
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
	// command_ids handed to an executor at least once for the current plan (protocol.toml: an event
	// is feedback for a delegated command). Cleared for an intent when its plan is replaced.
	dispatched map[string]struct{}
	seq        atomic.Uint64
	retry      CommandRetryPolicy
	// Backoff sleeper and clock; replaceable in tests for deterministic retry timing.
	sleep func(ctx context.Context, d time.Duration) error
	now   func() time.Time
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
		desired:    make(map[string]DesiredIntent),
		observed:   make(map[string]*ObservedIntent),
		executors:  make(map[string]CommandExecutor),
		seedLocks:  make(map[string]seedLock),
		dispatched: make(map[string]struct{}),
		retry:      DefaultCommandRetryPolicy(),
		sleep:      sleepContext,
		now:        time.Now,
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
	stages := normalizeIssueToStages(issue)
	commands := flattenStagesToPlannedCommands(issue.IntentID, stages)
	issue.SeedDependencies = seedDependenciesForCommands(commands)
	o.mu.Lock()
	defer o.mu.Unlock()
	// command_id must resolve to exactly one intent; sanitized intent ids can collide ("a/b" vs "a.b").
	for i := range commands {
		id := commands[i].Command.CommandID
		if owner, _, ok := o.findPlannedCommandByCommandID(id); ok && owner != issue.IntentID {
			return fmt.Errorf("%w: command_id %q collides with intent %q", ErrInvalidIssue, id, owner)
		}
	}
	// The actor is the caller's TLS principal (admin_security.toml [authorization] actor_source);
	// an existing intent may only be replaced by the actor that submitted it.
	if prior, ok := o.desired[issue.IntentID]; ok && strings.TrimSpace(prior.Issue.Actor) != strings.TrimSpace(issue.Actor) {
		return fmt.Errorf("%w: intent %q was submitted by %q", ErrIntentActorMismatch, issue.IntentID, prior.Issue.Actor)
	}
	// A replaced plan has not been dispatched, even when it reuses the same command_ids.
	for _, prior := range o.desired[issue.IntentID].Commands {
		delete(o.dispatched, prior.Command.CommandID)
	}
	o.desired[issue.IntentID] = DesiredIntent{
		Issue:      issue,
		Commands:   commands,
		ReceivedAt: now,
	}
	delete(o.observed, issue.IntentID)
	emitIssue(o.seq.Add(1), issue)
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
	out.PendingCount = pendingCommandCount(desired.Commands, obs)
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
		emitReport(o.seq.Add(1), desired.Issue.Actor, report)
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
				emitReport(o.seq.Add(1), desired.Issue.Actor, report)
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

	wireCommand, commandMsgID, err := o.dispatchCommandEnvelope(next.Command)
	if err != nil {
		if next.Blocking {
			o.releaseSeedLock(next.SeedKey, key, next.Command.CommandID)
		}
		return session.Report{}, err
	}
	// Recorded before the first attempt so a terminal event arriving over the session mid-dispatch is accepted.
	o.markDispatched(next.Command.CommandID)
	emitCommand(commandMsgID, wireCommand, observability.StatusDispatched, "")
	result := o.dispatchWithRetry(ctx, key, exec, wireCommand, commandMsgID)

	o.mu.Lock()
	observed := ensureObservedLocked(o, key)
	cmdID := next.Command.CommandID

	// A terminal event already arrived through the session path; it owns observation and lock release.
	if result.Observed || observed.ByCommandID[cmdID].EventID != "" {
		report := latestReportOrSynthesizeComplete(desired, observed)
		o.mu.Unlock()
		return report, nil
	}

	// No terminal event: record timed_out/retry_exhausted and close the command with a failed report.
	if !result.HasEvent {
		observed.CommandStates[cmdID] = result.Status
		report := buildCommandTerminalReport(desired, next, result.Status)
		observed.Reports = append(observed.Reports, report)
		observed.ObservedAt = o.now()
		if next.Blocking {
			delete(o.seedLocks, next.SeedKey)
		}
		o.mu.Unlock()
		emitReport(o.seq.Add(1), desired.Issue.Actor, report)
		return report, nil
	}

	report, ingestedEvent, eventMsgID, reportMsgID, err := o.ingestEventEnvelopeAndBuildReport(desired, observed, result.Event)
	if err != nil {
		o.mu.Unlock()
		if next.Blocking {
			o.releaseSeedLock(next.SeedKey, key, next.Command.CommandID)
		}
		return session.Report{}, err
	}
	observed.ByCommandID[ingestedEvent.CommandID] = ingestedEvent
	observed.CommandStates[ingestedEvent.CommandID] = result.Status
	observed.Events = append(observed.Events, ingestedEvent)
	observed.Reports = append(observed.Reports, report)
	observed.ObservedAt = o.now()
	if next.Blocking {
		delete(o.seedLocks, next.SeedKey)
	}
	o.mu.Unlock()
	emitEventIn(eventMsgID, ingestedEvent.GhostID, ingestedEvent, ingestedEvent.Outcome, "")
	emitReport(reportMsgID, desired.Issue.Actor, report)
	return report, nil
}

// IngestObservedEvent maps an external ghost event into intent observed state.
// The command's planned ghost is checked against boundGhostID under the same lock and against
// the same plan entry used for observation; a mismatch returns ErrCommandGhostMismatch with no
// mutation. A planned command that was never dispatched returns ErrCommandNotDispatched with no
// mutation: an unsolicited event must not complete it. duplicate=true means the command was
// already observed; observed state is not re-mutated and the latest report is returned for
// read-only emission.
func (o *Orchestrator) IngestObservedEvent(boundGhostID string, event session.Event) (session.Report, bool, bool, error) {
	return o.ingestObservedEvent(boundGhostID, event, 0)
}

// ingestObservedEvent is IngestObservedEvent with the inbound event frame message_id (0 when not
// framed by a session) so the custody event line is logged before the report it produces.
func (o *Orchestrator) ingestObservedEvent(boundGhostID string, event session.Event, eventMessageID uint64) (session.Report, bool, bool, error) {
	o.mu.Lock()
	defer o.mu.Unlock()

	intentID, plan, ok := o.findPlannedCommandByCommandID(event.CommandID)
	if !ok {
		return session.Report{}, false, false, nil
	}
	if planned := strings.TrimSpace(plan.Command.GhostID); planned != strings.TrimSpace(boundGhostID) {
		return session.Report{}, true, false, fmt.Errorf(
			"%w: command_id=%q planned for ghost_id=%q, not %q",
			ErrCommandGhostMismatch, event.CommandID, planned, strings.TrimSpace(boundGhostID),
		)
	}
	if _, sent := o.dispatched[event.CommandID]; !sent {
		return session.Report{}, true, false, fmt.Errorf(
			"%w: command_id=%q is planned but was never sent to ghost_id=%q",
			ErrCommandNotDispatched, event.CommandID, strings.TrimSpace(plan.Command.GhostID),
		)
	}
	desired := o.desired[intentID]
	observed := ensureObservedLocked(o, intentID)
	// Duplicate observation: no report is emitted; the session boundary logs the duplicate.
	if _, exists := observed.ByCommandID[event.CommandID]; exists {
		if len(observed.Reports) == 0 {
			return latestReportOrSynthesizeComplete(desired, observed), true, true, nil
		}
		return observed.Reports[len(observed.Reports)-1], true, true, nil
	}

	report, ingestedEvent, eventMsgID, reportMsgID, err := o.ingestEventEnvelopeAndBuildReport(desired, observed, event)
	if err != nil {
		return session.Report{}, true, false, err
	}
	observed.ByCommandID[ingestedEvent.CommandID] = ingestedEvent
	// A real terminal event supersedes an inferred timed_out/retry_exhausted; attempts are kept.
	prior := observed.CommandStates[ingestedEvent.CommandID]
	observed.CommandStates[ingestedEvent.CommandID] = statusFromEvent(ingestedEvent, prior.Attempts, o.now())
	observed.Events = append(observed.Events, ingestedEvent)
	observed.Reports = append(observed.Reports, report)
	observed.ObservedAt = o.now()
	if plan.Blocking {
		delete(o.seedLocks, plan.SeedKey)
	}
	if eventMessageID == 0 {
		eventMessageID = eventMsgID
	}
	emitEventIn(eventMessageID, boundGhostID, ingestedEvent, ingestedEvent.Outcome, "")
	emitReport(reportMsgID, desired.Issue.Actor, report)
	return report, true, false, nil
}

// Records that command_id is being handed to an executor; session events for it are accepted from here on.
func (o *Orchestrator) markDispatched(commandID string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.dispatched[commandID] = struct{}{}
}

// dispatchCommandEnvelope round-trips command payload through protocol framing and returns the
// frame message_id used for custody logging.
func (o *Orchestrator) dispatchCommandEnvelope(command session.Command) (session.Command, uint64, error) {
	messageID := o.seq.Add(1)
	payload, err := session.EncodeCommandFrame(messageID, command)
	if err != nil {
		return session.Command{}, 0, err
	}
	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		return session.Command{}, 0, err
	}
	cmd, err := session.DecodeCommandFrame(fr)
	return cmd, messageID, err
}

// ingestEventEnvelopeAndBuildReport ingests a framed event and emits a framed report; returns the
// event and report frame message_ids for custody logging.
func (o *Orchestrator) ingestEventEnvelopeAndBuildReport(
	desired DesiredIntent,
	observed *ObservedIntent,
	event session.Event,
) (session.Report, session.Event, uint64, uint64, error) {
	eventMsgID := o.seq.Add(1)
	eventPayload, err := session.EncodeEventFrame(eventMsgID, event)
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}
	eventFrame, err := frame.ReadFrame(bytes.NewReader(eventPayload), frame.DefaultLimits())
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}
	ingestedEvent, err := session.DecodeEventFrame(eventFrame)
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}

	report := buildReportFromObserved(desired, observed, ingestedEvent)
	reportMsgID := o.seq.Add(1)
	reportPayload, err := session.EncodeReportFrame(reportMsgID, report)
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}
	reportFrame, err := frame.ReadFrame(bytes.NewReader(reportPayload), frame.DefaultLimits())
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}
	wireReport, err := session.DecodeReportFrame(reportFrame)
	if err != nil {
		return session.Report{}, session.Event{}, 0, 0, err
	}
	return wireReport, ingestedEvent, eventMsgID, reportMsgID, nil
}

// normalizeIssueToStages assigns default IDs to issue stages.
func normalizeIssueToStages(issue IssueEnv) []IssueStage {
	out := make([]IssueStage, 0, len(issue.Stages))
	for i := range issue.Stages {
		stage := issue.Stages[i]
		if strings.TrimSpace(stage.ID) == "" {
			stage.ID = fmt.Sprintf("stage.%d", i+1)
		}
		out = append(out, stage)
	}
	return out
}

func flattenStagesToPlannedCommands(
	intentID string,
	stages []IssueStage,
) []PlannedCommand {
	var out []PlannedCommand
	seq := 0

	for si := range stages {
		stage := stages[si]
		for ci := range stage.Commands {
			seq++
			step := stage.Commands[ci]

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

	return out
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
		Summary:         fmt.Sprintf("intent %s has no planned commands", desired.Issue.IntentID),
		CompletionState: CompletionSatisfied,
		TimestampMS:     uint64(time.Now().UnixMilli()),
	}
}

// Whether a command is settled: terminal event observed or a terminal lifecycle state recorded.
func commandSettled(observed *ObservedIntent, commandID string) bool {
	if observed == nil {
		return false
	}
	if _, seen := observed.ByCommandID[commandID]; seen {
		return true
	}
	status, ok := observed.CommandStates[commandID]
	return ok && status.Terminal()
}

// nextPendingCommand returns the first command not yet settled for an intent.
func nextPendingCommand(commands []PlannedCommand, observed *ObservedIntent) (PlannedCommand, bool) {
	for i := range commands {
		if !commandSettled(observed, commands[i].Command.CommandID) {
			return commands[i], true
		}
	}
	return PlannedCommand{}, false
}

// pendingCommandCount returns desired commands not yet settled.
func pendingCommandCount(commands []PlannedCommand, observed *ObservedIntent) int {
	out := 0
	for i := range commands {
		if !commandSettled(observed, commands[i].Command.CommandID) {
			out++
		}
	}
	return out
}

// Report for a command that ended without a terminal event (timed_out or retry_exhausted).
func buildCommandTerminalReport(desired DesiredIntent, next PlannedCommand, status CommandStatus) session.Report {
	return session.Report{
		IntentID: desired.Issue.IntentID,
		Phase:    ReportPhaseComplete,
		Summary: fmt.Sprintf(
			"intent %s command %s %s after %d attempt(s) on %s: %s",
			desired.Issue.IntentID,
			next.Command.CommandID,
			status.State,
			status.Attempts,
			next.Command.GhostID,
			status.LastError,
		),
		CompletionState: CompletionFailed,
		CommandID:       next.Command.CommandID,
		Outcome:         OutcomeError,
		TimestampMS:     uint64(status.UpdatedAt.UnixMilli()),
	}
}

// cloneObserved returns a defensive copy of observed state maps/slices.
func cloneObserved(in ObservedIntent) ObservedIntent {
	out := ObservedIntent{
		Events:        append([]session.Event{}, in.Events...),
		Reports:       append([]session.Report{}, in.Reports...),
		ObservedAt:    in.ObservedAt,
		ByCommandID:   make(map[string]session.Event, len(in.ByCommandID)),
		CommandStates: make(map[string]CommandStatus, len(in.CommandStates)),
	}
	maps.Copy(out.ByCommandID, in.ByCommandID)
	maps.Copy(out.CommandStates, in.CommandStates)
	return out
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
	observed = &ObservedIntent{
		ByCommandID:   make(map[string]session.Event),
		CommandStates: make(map[string]CommandStatus),
	}
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
