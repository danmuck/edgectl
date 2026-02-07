package ghost

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestServerLifecycleHappyPath(t *testing.T) {
	testlog.Start(t)
	s := NewServer()

	initial := s.Status()
	if initial.Phase != PhaseBoot {
		t.Fatalf("unexpected initial phase: %s", initial.Phase)
	}

	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
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

	got := s.Status()
	if got.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected ghost id: %q", got.GhostID)
	}
	if got.Phase != PhaseRadiating {
		t.Fatalf("unexpected phase: %s", got.Phase)
	}
	if got.SeedCount != 1 {
		t.Fatalf("unexpected seed count: %d", got.SeedCount)
	}
}

func TestServerAppearValidation(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{}); !errors.Is(err, ErrInvalidGhostID) {
		t.Fatalf("expected ErrInvalidGhostID, got %v", err)
	}
}

func TestServerLifecycleOrder(t *testing.T) {
	testlog.Start(t)
	s := NewServer()

	if err := s.Radiate(); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder from radiate-before-appear, got %v", err)
	}

	if err := s.Seed(seeds.NewRegistry()); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder from seed-before-appear, got %v", err)
	}
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	if err := s.Radiate(); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder from radiate-before-seed, got %v", err)
	}
}

func TestServerSeedAfterRadiateRejected(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	if err := s.Seed(seeds.NewRegistry()); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if err := s.Radiate(); err != nil {
		t.Fatalf("radiate failed: %v", err)
	}
	if err := s.Seed(seeds.NewRegistry()); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder from seed-after-radiate, got %v", err)
	}
}

func TestServerSeedNilRegistry(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	if err := s.Seed(nil); !errors.Is(err, ErrSeedRegistry) {
		t.Fatalf("expected ErrSeedRegistry, got %v", err)
	}
}

func TestServerRadiateWithEmptyRegistryAllowed(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	empty := seeds.NewRegistry()
	if err := s.Seed(empty); err != nil {
		t.Fatalf("seed failed: %v", err)
	}
	if err := s.Radiate(); err != nil {
		t.Fatalf("radiate failed: %v", err)
	}
	got := s.Status()
	if got.Phase != PhaseRadiating {
		t.Fatalf("unexpected phase: %s", got.Phase)
	}
	if got.SeedCount != 0 {
		t.Fatalf("expected seed count 0, got %d", got.SeedCount)
	}
}
