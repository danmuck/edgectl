package mirage

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeExecutor struct {
	count int
}

func (e *fakeExecutor) ExecuteCommand(_ context.Context, cmd session.Command) (session.Event, error) {
	e.count++
	outcome := OutcomeSuccess
	if cmd.Operation == "fail" {
		outcome = OutcomeError
	}
	return session.Event{
		EventID:     fmt.Sprintf("evt.%s", cmd.CommandID),
		CommandID:   cmd.CommandID,
		IntentID:    cmd.IntentID,
		GhostID:     cmd.GhostID,
		SeedID:      cmd.SeedSelector,
		Outcome:     outcome,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}, nil
}

func TestOrchestratorMultiCommandProgressAndComplete(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()
	exec := &fakeExecutor{}
	if err := loop.RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.1",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "ordered rollout",
		CommandPlan: []IssueCommand{
			{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
			{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
		},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}

	repA, err := loop.ReconcileOnce(context.Background(), "intent.1")
	if err != nil {
		t.Fatalf("reconcile A: %v", err)
	}
	if repA.Phase != ReportPhaseInProgress {
		t.Fatalf("expected in_progress after first command, got %q", repA.Phase)
	}

	repB, err := loop.ReconcileOnce(context.Background(), "intent.1")
	if err != nil {
		t.Fatalf("reconcile B: %v", err)
	}
	if repB.Phase != ReportPhaseComplete {
		t.Fatalf("expected complete after second command, got %q", repB.Phase)
	}
	if repB.CompletionState != CompletionSatisfied {
		t.Fatalf("expected satisfied completion, got %q", repB.CompletionState)
	}
}

func TestOrchestratorBlockingSeedLockAcrossIntents(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()
	exec := &fakeExecutor{}
	if err := loop.RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.lock.holder",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "holder",
		CommandPlan: []IssueCommand{
			{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status", Blocking: true},
		},
	}); err != nil {
		t.Fatalf("submit holder: %v", err)
	}
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.lock.waiter",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "waiter",
		CommandPlan: []IssueCommand{
			{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status", Blocking: true},
		},
	}); err != nil {
		t.Fatalf("submit waiter: %v", err)
	}

	// Manually hold lock to simulate ordering requirement across intents.
	loop.mu.Lock()
	loop.seedLocks[seedLockKey("ghost.alpha", "seed.flow")] = seedLock{
		IntentID:  "intent.lock.holder",
		CommandID: "cmd.intent.lock.holder.1",
	}
	loop.mu.Unlock()

	rep, err := loop.ReconcileOnce(context.Background(), "intent.lock.waiter")
	if err != nil {
		t.Fatalf("reconcile waiter while locked: %v", err)
	}
	if rep.Phase != ReportPhaseInProgress {
		t.Fatalf("expected in_progress while blocked, got %q", rep.Phase)
	}
	if rep.CompletionState != CompletionInProgress {
		t.Fatalf("expected completion in_progress while blocked, got %q", rep.CompletionState)
	}
}

func TestOrchestratorIngestObservedEventByCommandID(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.observe.1",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "status",
		CommandPlan: []IssueCommand{
			{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
		},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}

	event := session.Event{
		EventID:     "evt.cmd.intent.observe.1.1",
		CommandID:   "cmd.intent.observe.1.1",
		IntentID:    "intent.observe.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	report, matched, err := loop.IngestObservedEvent(event)
	if err != nil {
		t.Fatalf("ingest observed event: %v", err)
	}
	if !matched {
		t.Fatalf("expected matched=true")
	}
	if report.Phase != ReportPhaseComplete {
		t.Fatalf("unexpected report phase: %q", report.Phase)
	}
}
