package ghost

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestHandleCommandAcceptedCreatesExecutionState(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	cmd := CommandEnv{
		MessageID:    101,
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
		Args:         map[string]string{"key": "value"},
	}

	state, err := s.HandleCommand(cmd)
	if err != nil {
		t.Fatalf("handle command failed: %v", err)
	}

	if state.Phase != ExecutionAccepted {
		t.Fatalf("unexpected phase: %s", state.Phase)
	}
	if state.ExecutionID != "exec.cmd.1" {
		t.Fatalf("unexpected execution id: %q", state.ExecutionID)
	}
	if state.CommandID != "cmd.1" || state.MessageID != 101 {
		t.Fatalf("unexpected state IDs: %+v", state)
	}

	byCmd, ok := s.ExecutionByCommandID("cmd.1")
	if !ok || byCmd.ExecutionID != state.ExecutionID {
		t.Fatalf("execution by command missing or mismatched: ok=%v state=%+v", ok, byCmd)
	}

	byMsg, ok := s.ExecutionByMessageID(101)
	if !ok || byMsg.ExecutionID != state.ExecutionID {
		t.Fatalf("execution by message missing or mismatched: ok=%v state=%+v", ok, byMsg)
	}

	cmd.Args["key"] = "changed"
	if state.Args["key"] != "value" {
		t.Fatalf("execution args should be cloned, got %q", state.Args["key"])
	}
}

func TestHandleCommandRequiresRadiating(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	if err := s.Seed(seeds.NewRegistry()); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	_, err := s.HandleCommand(CommandEnv{
		MessageID:    1,
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrNotRadiating) {
		t.Fatalf("expected ErrNotRadiating, got %v", err)
	}
}

func TestHandleCommandValidation(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	_, err := s.HandleCommand(CommandEnv{
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrInvalidCommandEnv) {
		t.Fatalf("expected ErrInvalidCommandEnv, got %v", err)
	}
}

func TestHandleCommandTargetMismatch(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	_, err := s.HandleCommand(CommandEnv{
		MessageID:    1,
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.beta",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrCommandTargetMismatch) {
		t.Fatalf("expected ErrCommandTargetMismatch, got %v", err)
	}
}

func TestHandleCommandDuplicateIDs(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	_, err := s.HandleCommand(CommandEnv{
		MessageID:    1,
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("initial command failed: %v", err)
	}

	_, err = s.HandleCommand(CommandEnv{
		MessageID:    2,
		CommandID:    "cmd.1",
		IntentID:     "intent.2",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf("expected ErrDuplicateCommandID, got %v", err)
	}

	_, err = s.HandleCommand(CommandEnv{
		MessageID:    1,
		CommandID:    "cmd.2",
		IntentID:     "intent.2",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrDuplicateMessageID) {
		t.Fatalf("expected ErrDuplicateMessageID, got %v", err)
	}
}

func newRadiatingServer(t *testing.T, ghostID string) *Server {
	t.Helper()
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: ghostID}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	reg := seeds.NewRegistry()
	if err := reg.Register(seedflow.NewSeed()); err != nil {
		t.Fatalf("register flow seed: %v", err)
	}
	if err := s.Seed(reg); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if err := s.Radiate(); err != nil {
		t.Fatalf("radiate failed: %v", err)
	}
	return s
}
