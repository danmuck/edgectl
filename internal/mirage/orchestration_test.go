package mirage

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	seedfs "github.com/danmuck/edgectl/internal/seeds/fs"
	seedkv "github.com/danmuck/edgectl/internal/seeds/kv"
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
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
				},
			},
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
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status", Blocking: true},
				},
			},
		},
	}); err != nil {
		t.Fatalf("submit holder: %v", err)
	}
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.lock.waiter",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "waiter",
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status", Blocking: true},
				},
			},
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
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
				},
			},
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

func TestOrchestratorSubmitIssueDerivesSeedDependencies(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:    "intent.deps.1",
		Actor:       "user:dan",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "derive deps",
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.fs", Operation: "write"},
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
					{GhostID: "ghost.alpha", SeedSelector: "seed.fs", Operation: "read"},
				},
			},
		},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}

	snap, ok := loop.SnapshotIntent("intent.deps.1")
	if !ok {
		t.Fatalf("expected intent snapshot")
	}
	deps := snap.Desired.Issue.SeedDependencies
	if len(deps) != 2 {
		t.Fatalf("unexpected derived deps: %+v", deps)
	}
	if deps[0] != "seed.flow" || deps[1] != "seed.fs" {
		t.Fatalf("unexpected derived deps order/content: %+v", deps)
	}
}

// multiSeedExecutor dispatches commands to seed.fs or seed.kv based on SeedSelector.
type multiSeedExecutor struct {
	ghostID string
	fs      seedfs.Seed
	kv      *seedkv.Seed
}

// ExecuteCommand routes one command to the matching seed and returns one event.
func (e *multiSeedExecutor) ExecuteCommand(_ context.Context, cmd session.Command) (session.Event, error) {
	var outcome string
	switch cmd.SeedSelector {
	case "seed.fs":
		result, err := e.fs.Execute(cmd.Operation, cmd.Args)
		outcome = OutcomeSuccess
		if err != nil || result.Status != "ok" || result.ExitCode != 0 {
			outcome = OutcomeError
		}
	case "seed.kv":
		result, err := e.kv.Execute(cmd.Operation, cmd.Args)
		outcome = OutcomeSuccess
		if err != nil || result.Status != "ok" || result.ExitCode != 0 {
			outcome = OutcomeError
		}
	default:
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
		Stages: []IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []IssueCommand{
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
		Stages: []IssueStage{
			{
				ID:       "stage.1",
				Barrier:  true,
				Commands: commands,
			},
		},
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

// E2E: multi-seed, multi-stage intent using explicit Stages.
// Stage 1 writes a file via seed.fs on ghost.alpha (barrier).
// Stage 2 indexes file metadata via seed.kv on both ghosts (barrier).
// Verifies: stage ordering, cross-seed orchestration, real seed state.
func TestOrchestratorE2EMultiSeedStoreAndIndex(t *testing.T) {
	testlog.Start(t)

	loop := NewOrchestrator()

	kvAlpha := seedkv.NewSeed()
	kvBeta := seedkv.NewSeed()
	fsRoot := t.TempDir()

	executors := map[string]*multiSeedExecutor{
		"ghost.alpha": {
			ghostID: "ghost.alpha",
			fs:      seedfs.NewSeedWithRoot(fsRoot),
			kv:      kvAlpha,
		},
		"ghost.beta": {
			ghostID: "ghost.beta",
			fs:      seedfs.NewSeedWithRoot(t.TempDir()),
			kv:      kvBeta,
		},
	}
	for ghostID, exec := range executors {
		if err := loop.RegisterExecutor(ghostID, exec); err != nil {
			t.Fatalf("register executor %s: %v", ghostID, err)
		}
	}

	// Submit multi-stage issue using explicit stages.
	// This mirrors what a properly wired orchestrator template would produce.
	if err := loop.SubmitIssue(IssueEnv{
		IntentID:         "intent.multiseed.1",
		Actor:            "user:test",
		TargetScope:      "ghost:ghost.alpha",
		Objective:        "store file then index metadata across seeds",
		SeedDependencies: []string{"seed.fs", "seed.kv"},
		Stages: []IssueStage{
			{
				ID:      "fs-write",
				Barrier: true,
				Commands: []IssueCommand{
					{
						GhostID:      "ghost.alpha",
						SeedSelector: "seed.fs",
						Operation:    "write",
						Args: map[string]string{
							"path":    "artifacts/report.txt",
							"content": "multi-seed-e2e-payload",
						},
						Blocking: true,
					},
				},
			},
			{
				ID:      "kv-index",
				Barrier: true,
				Commands: []IssueCommand{
					{
						GhostID:      "ghost.alpha",
						SeedSelector: "seed.kv",
						Operation:    "put",
						Args: map[string]string{
							"key":   "index:artifacts/report.txt",
							"value": "ghost=ghost.alpha,path=artifacts/report.txt",
						},
						Blocking: true,
					},
					{
						GhostID:      "ghost.beta",
						SeedSelector: "seed.kv",
						Operation:    "put",
						Args: map[string]string{
							"key":   "index:artifacts/report.txt",
							"value": "ghost=ghost.alpha,path=artifacts/report.txt",
						},
						Blocking: true,
					},
				},
			},
		},
	}); err != nil {
		t.Fatalf("submit multi-seed issue: %v", err)
	}

	// Verify desired state: 3 planned commands (1 fs + 2 kv).
	snap, ok := loop.SnapshotIntent("intent.multiseed.1")
	if !ok {
		t.Fatalf("expected intent snapshot")
	}
	if len(snap.Desired.Commands) != 3 {
		t.Fatalf("expected 3 planned commands, got %d", len(snap.Desired.Commands))
	}
	if snap.PendingCount != 3 {
		t.Fatalf("expected 3 pending, got %d", snap.PendingCount)
	}

	// Verify derived seed dependencies include both seeds.
	deps := snap.Desired.Issue.SeedDependencies
	if len(deps) != 2 {
		t.Fatalf("expected 2 seed dependencies, got %+v", deps)
	}

	// Reconcile pass 1: stage 1 seed.fs write.
	rep1, err := loop.ReconcileOnce(context.Background(), "intent.multiseed.1")
	if err != nil {
		t.Fatalf("reconcile pass 1: %v", err)
	}
	if rep1.Phase != ReportPhaseInProgress {
		t.Fatalf("expected in_progress after stage 1, got %q", rep1.Phase)
	}

	// Verify file was written to disk.
	content, err := os.ReadFile(filepath.Join(fsRoot, "artifacts", "report.txt"))
	if err != nil {
		t.Fatalf("read written file: %v", err)
	}
	if string(content) != "multi-seed-e2e-payload" {
		t.Fatalf("unexpected file content: %q", string(content))
	}

	// Reconcile pass 2: stage 2 first kv put (ghost.alpha).
	rep2, err := loop.ReconcileOnce(context.Background(), "intent.multiseed.1")
	if err != nil {
		t.Fatalf("reconcile pass 2: %v", err)
	}
	if rep2.Phase != ReportPhaseInProgress {
		t.Fatalf("expected in_progress after stage 2 cmd 1, got %q", rep2.Phase)
	}

	// Reconcile pass 3: stage 2 second kv put (ghost.beta) — terminal.
	rep3, err := loop.ReconcileOnce(context.Background(), "intent.multiseed.1")
	if err != nil {
		t.Fatalf("reconcile pass 3: %v", err)
	}
	if rep3.Phase != ReportPhaseComplete {
		t.Fatalf("expected complete after all stages, got %q", rep3.Phase)
	}
	if rep3.CompletionState != CompletionSatisfied {
		t.Fatalf("expected satisfied, got %q", rep3.CompletionState)
	}

	// Verify kv index state on both ghosts.
	for name, kv := range map[string]*seedkv.Seed{"alpha": kvAlpha, "beta": kvBeta} {
		result, err := kv.Execute("get", map[string]string{"key": "index:artifacts/report.txt"})
		if err != nil {
			t.Fatalf("kv get on %s: %v", name, err)
		}
		if result.Status != "ok" {
			t.Fatalf("kv get on %s status: %q", name, result.Status)
		}
		val := strings.TrimSpace(string(result.Stdout))
		if val != "ghost=ghost.alpha,path=artifacts/report.txt" {
			t.Fatalf("unexpected kv value on %s: %q", name, val)
		}
	}

	// Verify final snapshot reflects full completion.
	finalSnap, ok := loop.SnapshotIntent("intent.multiseed.1")
	if !ok {
		t.Fatalf("expected final snapshot")
	}
	if finalSnap.PendingCount != 0 {
		t.Fatalf("expected 0 pending after completion, got %d", finalSnap.PendingCount)
	}
	if !finalSnap.HasObserved {
		t.Fatalf("expected observed state")
	}
	if len(finalSnap.Observed.Events) != 3 {
		t.Fatalf("expected 3 observed events, got %d", len(finalSnap.Observed.Events))
	}
}
