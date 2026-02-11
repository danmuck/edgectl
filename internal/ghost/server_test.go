package ghost

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	seedkv "github.com/danmuck/edgectl/internal/seeds/kv"
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

func TestServerSeedCatalogDeterministicAndDefensive(t *testing.T) {
	testlog.Start(t)
	s := NewServer()
	if err := s.Appear(GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear failed: %v", err)
	}
	reg := seeds.NewRegistry()
	if err := reg.Register(seedkv.NewSeed()); err != nil {
		t.Fatalf("register kv seed: %v", err)
	}
	if err := reg.Register(seedflow.NewSeed()); err != nil {
		t.Fatalf("register flow seed: %v", err)
	}
	if err := s.Seed(reg); err != nil {
		t.Fatalf("seed failed: %v", err)
	}

	catalog := s.SeedCatalog()
	if len(catalog) != 2 {
		t.Fatalf("unexpected catalog size: %d", len(catalog))
	}
	if catalog[0].Metadata.ID != "seed.flow" || catalog[1].Metadata.ID != "seed.kv" {
		t.Fatalf("unexpected seed catalog order: %+v", catalog)
	}
	if len(catalog[0].Operations) == 0 || len(catalog[0].CommandCatalog) == 0 {
		t.Fatalf("expected flow capability details, got: %+v", catalog[0])
	}
	if len(catalog[1].Operations) == 0 || len(catalog[1].CommandCatalog) == 0 {
		t.Fatalf("expected kv capability details, got: %+v", catalog[1])
	}

	// Mutate returned snapshot and ensure next read remains unchanged.
	catalog[0].Metadata.ID = "mutated"
	catalog[0].Operations[0].Name = "mutated-op"
	catalog[1].CommandCatalog[0].Label = "mutated-label"
	if len(catalog[1].CommandCatalog[0].Args) > 0 {
		catalog[1].CommandCatalog[0].Args[0].Key = "mutated-arg"
	}

	next := s.SeedCatalog()
	if next[0].Metadata.ID != "seed.flow" {
		t.Fatalf("metadata mutation leaked into server state: %+v", next[0].Metadata)
	}
	if next[0].Operations[0].Name == "mutated-op" {
		t.Fatalf("operation mutation leaked into server state: %+v", next[0].Operations)
	}
	if next[1].CommandCatalog[0].Label == "mutated-label" {
		t.Fatalf("command catalog mutation leaked into server state: %+v", next[1].CommandCatalog[0])
	}
}
