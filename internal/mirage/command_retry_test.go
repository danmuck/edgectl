package mirage

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Executor that fails a scripted number of times before returning a terminal event.
type scriptedExecutor struct {
	mu       sync.Mutex
	failures []error // consumed in order; when empty, returns a terminal event
	outcome  string
	seen     []string // command_id per attempt
}

func (e *scriptedExecutor) ExecuteCommand(ctx context.Context, cmd session.Command) (session.Event, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.seen = append(e.seen, cmd.CommandID)
	if len(e.failures) > 0 {
		err := e.failures[0]
		e.failures = e.failures[1:]
		return session.Event{}, err
	}
	outcome := e.outcome
	if outcome == "" {
		outcome = OutcomeSuccess
	}
	return session.Event{
		EventID:     "evt." + cmd.CommandID,
		CommandID:   cmd.CommandID,
		IntentID:    cmd.IntentID,
		GhostID:     cmd.GhostID,
		SeedID:      cmd.SeedSelector,
		Outcome:     outcome,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}, nil
}

func (e *scriptedExecutor) attempts() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string{}, e.seen...)
}

// Records backoff delays instead of sleeping; optional hook runs during the "sleep".
type recordingSleeper struct {
	delays []time.Duration
	hook   func()
	err    error
}

func (r *recordingSleeper) sleep(ctx context.Context, d time.Duration) error {
	r.delays = append(r.delays, d)
	if r.hook != nil {
		r.hook()
	}
	return r.err
}

const retryIntent = "intent.retry.1"
const retryCommand = "cmd.intent.retry.1.1"

func newRetryOrchestrator(t *testing.T, exec CommandExecutor, sleeper *recordingSleeper) *Orchestrator {
	t.Helper()
	o := NewOrchestrator()
	o.sleep = sleeper.sleep
	if err := o.RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	if err := o.SubmitIssue(IssueEnv{
		IntentID:    retryIntent,
		Actor:       "user:test",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "status",
		Stages: []IssueStage{{
			ID:       "stage.1",
			Barrier:  true,
			Commands: []IssueCommand{{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"}},
		}},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}
	return o
}

func commandStatus(t *testing.T, o *Orchestrator) CommandStatus {
	t.Helper()
	snap, ok := o.SnapshotIntent(retryIntent)
	if !ok {
		t.Fatalf("intent missing")
	}
	status, ok := snap.Observed.CommandStates[retryCommand]
	if !ok {
		t.Fatalf("no command state recorded; states=%+v", snap.Observed.CommandStates)
	}
	return status
}

var errTransport = errors.New("dial tcp: connection refused")

func TestCommandRetrySucceedsAfterTransientFailuresWithSameCommandID(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport}}
	sleeper := &recordingSleeper{}
	o := newRetryOrchestrator(t, exec, sleeper)

	report, err := o.ReconcileOnce(context.Background(), retryIntent)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.CompletionState != CompletionSatisfied {
		t.Fatalf("completion = %q, want %q", report.CompletionState, CompletionSatisfied)
	}

	seen := exec.attempts()
	if len(seen) != 3 {
		t.Fatalf("attempts = %d, want 3", len(seen))
	}
	for i, id := range seen {
		if id != retryCommand {
			t.Fatalf("attempt %d used command_id %q, want %q (idempotency key must be preserved)", i+1, id, retryCommand)
		}
	}

	status := commandStatus(t, o)
	if status.State != CommandComplete || status.Attempts != 3 {
		t.Fatalf("status = %+v, want complete after 3 attempts", status)
	}

	// Contract backoff: attempt N base = 250ms * 2^(N-1), jittered to [0.5, 1.5).
	if len(sleeper.delays) != 2 {
		t.Fatalf("backoff sleeps = %d, want 2", len(sleeper.delays))
	}
	bounds := [][2]time.Duration{{125 * time.Millisecond, 375 * time.Millisecond}, {250 * time.Millisecond, 750 * time.Millisecond}}
	for i, d := range sleeper.delays {
		if d < bounds[i][0] || d >= bounds[i][1] {
			t.Fatalf("backoff %d = %v, want in [%v, %v)", i+1, d, bounds[i][0], bounds[i][1])
		}
	}
}

func TestCommandRetryExhaustedIsTerminal(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport, errTransport, errTransport}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})

	report, err := o.ReconcileOnce(context.Background(), retryIntent)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Phase != ReportPhaseComplete || report.CompletionState != CompletionFailed {
		t.Fatalf("report = %+v, want complete/failed", report)
	}
	status := commandStatus(t, o)
	if status.State != CommandRetryExhausted || status.Attempts != DefaultCommandMaxAttempts {
		t.Fatalf("status = %+v, want retry_exhausted after %d", status, DefaultCommandMaxAttempts)
	}
	if status.LastError == "" {
		t.Fatalf("expected last error recorded")
	}

	// Terminal: a later reconcile must not dispatch again.
	again, err := o.ReconcileOnce(context.Background(), retryIntent)
	if err != nil {
		t.Fatalf("second reconcile: %v", err)
	}
	if got := len(exec.attempts()); got != DefaultCommandMaxAttempts {
		t.Fatalf("executor called %d times after terminal state, want %d", got, DefaultCommandMaxAttempts)
	}
	if again.CompletionState != CompletionFailed {
		t.Fatalf("second reconcile completion = %q, want failed", again.CompletionState)
	}
	snap, _ := o.SnapshotIntent(retryIntent)
	if snap.PendingCount != 0 {
		t.Fatalf("pending = %d, want 0 after terminal state", snap.PendingCount)
	}
}

func TestCommandRetryPolicyMaxAttemptsIsConfigurable(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport, errTransport, errTransport, errTransport}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})
	o.SetCommandRetryPolicy(CommandRetryPolicy{MaxAttempts: 5})

	if _, err := o.ReconcileOnce(context.Background(), retryIntent); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if status := commandStatus(t, o); status.State != CommandRetryExhausted || status.Attempts != 5 {
		t.Fatalf("status = %+v, want retry_exhausted after 5", status)
	}
}

func TestCommandTimedOutWhenDeadlineExpires(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{context.DeadlineExceeded}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})

	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()
	report, err := o.ReconcileOnce(ctx, retryIntent)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.CompletionState != CompletionFailed {
		t.Fatalf("completion = %q, want failed", report.CompletionState)
	}
	if status := commandStatus(t, o); status.State != CommandTimedOut || status.Attempts != 1 {
		t.Fatalf("status = %+v, want timed_out after 1 attempt", status)
	}
}

func TestCommandTimedOutWhenDeadlineExpiresDuringBackoff(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{err: context.DeadlineExceeded})

	if _, err := o.ReconcileOnce(context.Background(), retryIntent); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if status := commandStatus(t, o); status.State != CommandTimedOut || status.Attempts != 1 {
		t.Fatalf("status = %+v, want timed_out after 1 attempt", status)
	}
}

func TestCommandRetryStopsWhenTerminalEventObservedViaSession(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport}}
	sleeper := &recordingSleeper{}
	o := newRetryOrchestrator(t, exec, sleeper)

	// While Mirage backs off, the ghost's terminal event arrives over the session.
	sleeper.hook = func() {
		_, _, _, err := o.IngestObservedEvent("ghost.alpha", session.Event{
			EventID:     "evt." + retryCommand,
			CommandID:   retryCommand,
			IntentID:    retryIntent,
			GhostID:     "ghost.alpha",
			SeedID:      "seed.flow",
			Outcome:     OutcomeSuccess,
			TimestampMS: uint64(time.Now().UnixMilli()),
		})
		if err != nil {
			t.Errorf("session ingest: %v", err)
		}
	}

	report, err := o.ReconcileOnce(context.Background(), retryIntent)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if got := len(exec.attempts()); got != 1 {
		t.Fatalf("executor called %d times, want 1 (no retry after terminal event)", got)
	}
	if report.CompletionState != CompletionSatisfied {
		t.Fatalf("completion = %q, want satisfied", report.CompletionState)
	}
	snap, _ := o.SnapshotIntent(retryIntent)
	if len(snap.Observed.Reports) != 1 || len(snap.Observed.Events) != 1 {
		t.Fatalf("observed reports=%d events=%d, want 1/1", len(snap.Observed.Reports), len(snap.Observed.Events))
	}
}

func TestErrorOutcomeRecordsFailedState(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{outcome: OutcomeError}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})

	if _, err := o.ReconcileOnce(context.Background(), retryIntent); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if status := commandStatus(t, o); status.State != CommandFailed || status.Attempts != 1 {
		t.Fatalf("status = %+v, want failed after 1 attempt", status)
	}
}

func TestLateTerminalEventSupersedesRetryExhausted(t *testing.T) {
	testlog.Start(t)
	exec := &scriptedExecutor{failures: []error{errTransport, errTransport, errTransport}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})

	if _, err := o.ReconcileOnce(context.Background(), retryIntent); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if _, _, dup, err := o.IngestObservedEvent("ghost.alpha", session.Event{
		EventID:     "evt." + retryCommand,
		CommandID:   retryCommand,
		IntentID:    retryIntent,
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}); err != nil || dup {
		t.Fatalf("late ingest: dup=%v err=%v", dup, err)
	}
	status := commandStatus(t, o)
	if status.State != CommandComplete || status.Attempts != DefaultCommandMaxAttempts {
		t.Fatalf("status = %+v, want complete with attempts preserved", status)
	}
}
