package mirage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	seedfs "github.com/danmuck/edgectl/internal/seeds/fs"
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

type fsGhostExecutor struct {
	ghostID string
	seed    seedfs.Seed
}

// ExecuteCommand applies one command against a filesystem seed and returns one event.
func (e *fsGhostExecutor) ExecuteCommand(_ context.Context, cmd session.Command) (session.Event, error) {
	result, err := e.seed.Execute(cmd.Operation, cmd.Args)
	outcome := OutcomeSuccess
	if err != nil || result.Status != "ok" || result.ExitCode != 0 {
		outcome = OutcomeError
	}
	return session.Event{
		EventID:     fmt.Sprintf("evt.%s", cmd.CommandID),
		CommandID:   cmd.CommandID,
		IntentID:    cmd.IntentID,
		GhostID:     e.ghostID,
		SeedID:      cmd.SeedSelector,
		Outcome:     outcome,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}, nil
}

func TestOrchestratorControlLoopE2EStoreAndCopyToAllSeedFSGhosts(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()
	roots := map[string]string{
		"ghost.alpha": t.TempDir(),
		"ghost.beta":  t.TempDir(),
	}
	for ghostID, root := range roots {
		if err := loop.RegisterExecutor(ghostID, &fsGhostExecutor{
			ghostID: ghostID,
			seed:    seedfs.NewSeedWithRoot(root),
		}); err != nil {
			t.Fatalf("register executor %s: %v", ghostID, err)
		}
	}

	// Intent 1: store one file to a single ghost seed.fs instance.
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.store.1",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "store seed file",
		CommandPlan: []IssueCommand{
			{
				GhostID:      "ghost.alpha",
				SeedSelector: "seed.fs",
				Operation:    "write",
				Args: map[string]string{
					"path":    "payloads/source.txt",
					"content": "hello from mirage intent",
				},
				Blocking: true,
			},
		},
	}); err != nil {
		t.Fatalf("submit store intent: %v", err)
	}
	storeReport, err := loop.ReconcileOnce(context.Background(), "intent.store.1")
	if err != nil {
		t.Fatalf("reconcile store intent: %v", err)
	}
	if storeReport.Phase != ReportPhaseComplete || storeReport.CompletionState != CompletionSatisfied {
		t.Fatalf("unexpected store report: %+v", storeReport)
	}
	sourcePath := filepath.Join(roots["ghost.alpha"], "payloads", "source.txt")
	sourceContent, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("read source file: %v", err)
	}

	// Intent 2: copy file content to all ghosts that have seed.fs executors.
	ghostIDs := make([]string, 0, len(roots))
	for ghostID := range roots {
		ghostIDs = append(ghostIDs, ghostID)
	}
	sort.Strings(ghostIDs)
	commands := make([]IssueCommand, 0, len(ghostIDs))
	for _, ghostID := range ghostIDs {
		commands = append(commands, IssueCommand{
			GhostID:      ghostID,
			SeedSelector: "seed.fs",
			Operation:    "write",
			Args: map[string]string{
				"path":    "payloads/copied.txt",
				"content": string(sourceContent),
			},
			Blocking: true,
		})
	}
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.copy.all.1",
		Actor:       "user:dan",
		TargetScope: "ghost:all",
		Objective:   "copy to all seed.fs ghosts",
		CommandPlan: commands,
	}); err != nil {
		t.Fatalf("submit copy intent: %v", err)
	}

	copyReportA, err := loop.ReconcileOnce(context.Background(), "intent.copy.all.1")
	if err != nil {
		t.Fatalf("reconcile copy pass A: %v", err)
	}
	if copyReportA.Phase != ReportPhaseInProgress {
		t.Fatalf("expected in_progress after first copy command, got %+v", copyReportA)
	}
	copyReportB, err := loop.ReconcileOnce(context.Background(), "intent.copy.all.1")
	if err != nil {
		t.Fatalf("reconcile copy pass B: %v", err)
	}
	if copyReportB.Phase != ReportPhaseComplete || copyReportB.CompletionState != CompletionSatisfied {
		t.Fatalf("unexpected terminal copy report: %+v", copyReportB)
	}

	for _, ghostID := range ghostIDs {
		outPath := filepath.Join(roots[ghostID], "payloads", "copied.txt")
		out, err := os.ReadFile(outPath)
		if err != nil {
			t.Fatalf("read copied file for %s: %v", ghostID, err)
		}
		if string(out) != string(sourceContent) {
			t.Fatalf("unexpected copied content for %s: %q", ghostID, string(out))
		}
	}
}
