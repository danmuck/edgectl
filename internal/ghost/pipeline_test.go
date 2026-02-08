package ghost

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestHandleCommandAndExecuteSuccessEvent(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	event, err := s.HandleCommandAndExecute(CommandEnv{
		MessageID:    701,
		CommandID:    "cmd.701",
		IntentID:     "intent.701",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("handle and execute failed: %v", err)
	}
	if event.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected outcome: %q", event.Outcome)
	}
	if event.EventID != "evt.cmd.701" {
		t.Fatalf("unexpected event id: %q", event.EventID)
	}

	state, ok := s.GetExecution("exec.cmd.701")
	if !ok {
		t.Fatalf("execution state missing")
	}
	if state.Phase != ExecutionComplete {
		t.Fatalf("unexpected phase: %s", state.Phase)
	}
	if state.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected state outcome: %q", state.Outcome)
	}
	if state.SeedResult.ExitCode != 0 || state.SeedResult.Status != SeedStatusOK {
		t.Fatalf("unexpected seed result: %+v", state.SeedResult)
	}
}

func TestHandleCommandAndExecuteUnknownSeedEmitsErrorEvent(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServerWithRegistry(t, "ghost.alpha", seeds.NewRegistry())

	event, err := s.HandleCommandAndExecute(CommandEnv{
		MessageID:    702,
		CommandID:    "cmd.702",
		IntentID:     "intent.702",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.missing",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("handle and execute failed: %v", err)
	}
	if event.Outcome != OutcomeError {
		t.Fatalf("unexpected outcome: %q", event.Outcome)
	}

	state, ok := s.GetByCommandID("cmd.702")
	if !ok {
		t.Fatalf("execution state missing")
	}
	if state.SeedResult.Status != SeedStatusError {
		t.Fatalf("unexpected status: %q", state.SeedResult.Status)
	}
	if state.SeedResult.ExitCode != unknownSeedExitCode {
		t.Fatalf("unexpected exit code: %d", state.SeedResult.ExitCode)
	}
}

func TestHandleCommandAndExecuteUnknownActionEmitsErrorEvent(t *testing.T) {
	testlog.Start(t)
	s := newRadiatingServer(t, "ghost.alpha")

	event, err := s.HandleCommandAndExecute(CommandEnv{
		MessageID:    703,
		CommandID:    "cmd.703",
		IntentID:     "intent.703",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "not-real",
	})
	if err != nil {
		t.Fatalf("handle and execute failed: %v", err)
	}
	if event.Outcome != OutcomeError {
		t.Fatalf("unexpected outcome: %q", event.Outcome)
	}

	state, ok := s.ExecutionByMessageID(703)
	if !ok {
		t.Fatalf("execution state missing by message id")
	}
	if state.SeedResult.Status != SeedStatusError {
		t.Fatalf("unexpected seed status: %q", state.SeedResult.Status)
	}
	if state.Event.Outcome != OutcomeError {
		t.Fatalf("unexpected event outcome: %q", state.Event.Outcome)
	}
}

func TestHandleCommandAndExecuteBoundaryError(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}

	_, err := s.HandleCommandAndExecute(CommandEnv{
		MessageID:    704,
		CommandID:    "cmd.704",
		IntentID:     "intent.704",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if !errors.Is(err, ErrNotRadiating) {
		t.Fatalf("expected ErrNotRadiating, got %v", err)
	}
	if _, ok := s.ExecutionByCommandID("cmd.704"); ok {
		t.Fatalf("execution should not be recorded on boundary failure")
	}
}

func newRadiatingServerWithRegistry(t *testing.T, ghostID string, reg *seeds.Registry) *Server {
	t.Helper()
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: ghostID}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	if err := s.Seed(reg); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if err := s.Radiate(); err != nil {
		t.Fatalf("radiate failed: %v", err)
	}
	return s
}
