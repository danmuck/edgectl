package ghost

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestSingleCommandLoopEndToEndSuccess(t *testing.T) {
	testlog.Start(t)

	srv := newRadiatingServer(t, "ghost.alpha")
	loop := NewSingleCommandLoop()

	req := CommandRequest{
		RequestID: "request.1",
		Command: CommandEnv{
			IntentID:     "intent.1",
			GhostID:      "ghost.alpha",
			SeedSelector: "seed.flow",
			Operation:    "status",
		},
	}
	if err := loop.SubmitCommand(req); err != nil {
		t.Fatalf("submit command: %v", err)
	}

	before, ok := loop.SnapshotCommand(req.RequestID)
	if !ok {
		t.Fatalf("expected command snapshot")
	}
	if before.HasObserved {
		t.Fatalf("expected no observed state before reconcile")
	}

	report, err := loop.ReconcileOnce(srv, req.RequestID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Phase != ReportPhaseComplete {
		t.Fatalf("unexpected report phase: %q", report.Phase)
	}
	if report.CompletionState != CompletionSatisfied {
		t.Fatalf("unexpected completion_state: %q", report.CompletionState)
	}
	if report.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected outcome: %q", report.Outcome)
	}
	if report.CommandID == "" || report.ExecutionID == "" || report.EventID == "" {
		t.Fatalf("missing report correlation ids: %+v", report)
	}

	after, ok := loop.SnapshotCommand(req.RequestID)
	if !ok || !after.HasObserved {
		t.Fatalf("expected observed state after reconcile")
	}
	if after.Observed.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected observed outcome: %q", after.Observed.Outcome)
	}

	exec, ok := srv.ExecutionByCommandID(report.CommandID)
	if !ok {
		t.Fatalf("expected execution by command id")
	}
	if exec.SeedExecute.ExecutionID == "" || exec.SeedResult.ExecutionID == "" {
		t.Fatalf("expected seed.execute and seed.result in execution state: %+v", exec)
	}
	if exec.Event.EventID != report.EventID {
		t.Fatalf("event/report mismatch event_id=%q report=%q", exec.Event.EventID, report.EventID)
	}
}

func TestSingleCommandLoopEndToEndFailureUnknownSeed(t *testing.T) {
	testlog.Start(t)

	srv := newRadiatingServer(t, "ghost.alpha")
	loop := NewSingleCommandLoop()

	req := CommandRequest{
		RequestID: "request.2",
		Command: CommandEnv{
			IntentID:     "intent.2",
			GhostID:      "ghost.alpha",
			SeedSelector: "seed.missing",
			Operation:    "status",
		},
	}
	if err := loop.SubmitCommand(req); err != nil {
		t.Fatalf("submit command: %v", err)
	}

	report, err := loop.ReconcileOnce(srv, req.RequestID)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Phase != ReportPhaseComplete {
		t.Fatalf("unexpected report phase: %q", report.Phase)
	}
	if report.CompletionState != CompletionFailed {
		t.Fatalf("unexpected completion_state: %q", report.CompletionState)
	}
	if report.Outcome != OutcomeError {
		t.Fatalf("unexpected outcome: %q", report.Outcome)
	}
}

func TestSingleCommandLoopRejectsRepeatReconcile(t *testing.T) {
	testlog.Start(t)

	srv := newRadiatingServer(t, "ghost.alpha")
	loop := NewSingleCommandLoop()
	req := CommandRequest{
		RequestID: "request.3",
		Command: CommandEnv{
			IntentID:     "intent.3",
			GhostID:      "ghost.alpha",
			SeedSelector: "seed.flow",
			Operation:    "status",
		},
	}
	if err := loop.SubmitCommand(req); err != nil {
		t.Fatalf("submit command: %v", err)
	}
	if _, err := loop.ReconcileOnce(srv, req.RequestID); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if _, err := loop.ReconcileOnce(srv, req.RequestID); !errors.Is(err, ErrCommandAlreadyObserved) {
		t.Fatalf("expected ErrCommandAlreadyObserved, got %v", err)
	}
}
