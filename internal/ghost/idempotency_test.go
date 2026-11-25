package ghost

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Seed that counts executions so replay tests can assert no duplicate mutation.
type countingSeed struct {
	calls atomic.Int32
}

func (c *countingSeed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{ID: "seed.count", Name: "Count", Description: "counts executions"}
}

func (c *countingSeed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{{Name: "bump", Description: "increment counter"}}
}

func (c *countingSeed) CommandCatalog() []seeds.CommandTemplate { return nil }

func (c *countingSeed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	n := c.calls.Add(1)
	return seeds.SeedResult{Status: SeedStatusOK, Stdout: []byte{byte('0' + n)}}, nil
}

func newCountingServer(t *testing.T) (*Server, *countingSeed) {
	t.Helper()
	seed := &countingSeed{}
	reg := seeds.NewRegistry()
	if err := reg.Register(seed); err != nil {
		t.Fatalf("register counting seed: %v", err)
	}
	return newRadiatingServerWithRegistry(t, "ghost.alpha", reg), seed
}

func bumpCommand(messageID uint64) CommandEnv {
	return CommandEnv{
		MessageID:    messageID,
		CommandID:    "cmd.idem.1",
		IntentID:     "intent.idem.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.count",
		Operation:    "bump",
	}
}

func TestDuplicateCommandReplaysPriorEventWithoutReexecution(t *testing.T) {
	testlog.Start(t)
	s, seed := newCountingServer(t)

	first, err := s.HandleCommandAndExecute(bumpCommand(1))
	if err != nil {
		t.Fatalf("first execute: %v", err)
	}

	// Retry carries a fresh message_id but the same command_id.
	second, err := s.HandleCommandAndExecute(bumpCommand(2))
	if err != nil {
		t.Fatalf("replayed execute: %v", err)
	}

	if got := seed.calls.Load(); got != 1 {
		t.Fatalf("seed executed %d times, want 1", got)
	}
	if second != first {
		t.Fatalf("replay event differs:\nfirst=%+v\nsecond=%+v", first, second)
	}
	if _, ok := s.ExecutionByMessageID(2); ok {
		t.Fatalf("replay must not record a new message_id")
	}
}

func TestDuplicateSeedResultUnchangedByExecutionID(t *testing.T) {
	testlog.Start(t)
	s, _ := newCountingServer(t)

	if _, err := s.HandleCommandAndExecute(bumpCommand(1)); err != nil {
		t.Fatalf("first execute: %v", err)
	}
	before, ok := s.GetExecution("exec.cmd.idem.1")
	if !ok {
		t.Fatalf("execution missing")
	}
	if _, err := s.HandleCommandAndExecute(bumpCommand(2)); err != nil {
		t.Fatalf("replayed execute: %v", err)
	}
	after, _ := s.GetExecution("exec.cmd.idem.1")

	if string(after.SeedResult.Stdout) != string(before.SeedResult.Stdout) ||
		after.SeedResult.ExitCode != before.SeedResult.ExitCode ||
		after.SeedResult.Status != before.SeedResult.Status {
		t.Fatalf("seed.result mutated on replay:\nbefore=%+v\nafter=%+v", before.SeedResult, after.SeedResult)
	}
	if !after.CompletedAt.Equal(before.CompletedAt) {
		t.Fatalf("completion time mutated on replay")
	}
}

func TestDuplicateCommandInFlightIsRejected(t *testing.T) {
	testlog.Start(t)
	s, seed := newCountingServer(t)

	// Accepted but not yet complete: no terminal event exists to replay.
	if _, err := s.HandleCommand(bumpCommand(1)); err != nil {
		t.Fatalf("accept: %v", err)
	}
	_, err := s.HandleCommandAndExecute(bumpCommand(2))
	if !errors.Is(err, ErrDuplicateCommandID) {
		t.Fatalf("expected ErrDuplicateCommandID for in-flight duplicate, got %v", err)
	}
	if got := seed.calls.Load(); got != 0 {
		t.Fatalf("seed executed %d times, want 0", got)
	}
}

func TestDuplicateWindowRetainsAtLeastFifteenMinutes(t *testing.T) {
	testlog.Start(t)
	s, seed := newCountingServer(t)
	base := time.Now()
	s.now = func() time.Time { return base }

	if _, err := s.HandleCommandAndExecute(bumpCommand(1)); err != nil {
		t.Fatalf("first execute: %v", err)
	}

	s.now = func() time.Time { return base.Add(DuplicateWindow - time.Second) }
	if _, err := s.HandleCommandAndExecute(bumpCommand(2)); err != nil {
		t.Fatalf("replay inside window: %v", err)
	}
	if got := seed.calls.Load(); got != 1 {
		t.Fatalf("replay inside window re-executed: calls=%d", got)
	}

	s.now = func() time.Time { return base.Add(DuplicateWindow + time.Second) }
	if _, err := s.HandleCommandAndExecute(bumpCommand(3)); err != nil {
		t.Fatalf("execute after window: %v", err)
	}
	if got := seed.calls.Load(); got != 2 {
		t.Fatalf("expected re-execution after window expiry, calls=%d", got)
	}
	if _, ok := s.ExecutionByMessageID(1); ok {
		t.Fatalf("expired message_id index should be pruned")
	}
}
