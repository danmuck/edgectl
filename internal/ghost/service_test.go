package ghost

import (
	"errors"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestBuildBuiltinRegistryFlow(t *testing.T) {
	testlog.Start(t)
	reg, err := buildBuiltinRegistry([]string{"seed.flow"})
	if err != nil {
		t.Fatalf("build registry failed: %v", err)
	}
	list := reg.ListMetadata()
	if len(list) != 1 || list[0].ID != "seed.flow" {
		t.Fatalf("unexpected registry metadata: %+v", list)
	}
}

func TestBuildBuiltinRegistryMongod(t *testing.T) {
	testlog.Start(t)
	reg, err := buildBuiltinRegistry([]string{"seed.mongod"})
	if err != nil {
		t.Fatalf("build registry failed: %v", err)
	}
	list := reg.ListMetadata()
	if len(list) != 1 || list[0].ID != "seed.mongod" {
		t.Fatalf("unexpected registry metadata: %+v", list)
	}
}

func TestBuildBuiltinRegistryNone(t *testing.T) {
	testlog.Start(t)
	reg, err := buildBuiltinRegistry([]string{"none"})
	if err != nil {
		t.Fatalf("build registry failed: %v", err)
	}
	list := reg.ListMetadata()
	if len(list) != 0 {
		t.Fatalf("expected empty registry, got %+v", list)
	}
}

func TestBuildBuiltinRegistryUnknown(t *testing.T) {
	testlog.Start(t)
	_, err := buildBuiltinRegistry([]string{"seed.unknown"})
	if !errors.Is(err, ErrUnknownBuiltinSeed) {
		t.Fatalf("expected ErrUnknownBuiltinSeed, got %v", err)
	}
}

func TestServiceBootstrapWithFlowSeed(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: time.Second,
	})
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	status := svc.Server().Status()
	if status.Phase != PhaseRadiating {
		t.Fatalf("unexpected phase: %s", status.Phase)
	}
	if status.SeedCount != 1 {
		t.Fatalf("unexpected seed count: %d", status.SeedCount)
	}
}

func TestServiceBootstrapWithNoSeeds(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"none"},
		HeartbeatInterval: time.Second,
	})
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	status := svc.Server().Status()
	if status.Phase != PhaseRadiating {
		t.Fatalf("unexpected phase: %s", status.Phase)
	}
	if status.SeedCount != 0 {
		t.Fatalf("unexpected seed count: %d", status.SeedCount)
	}
}

func TestServiceBootstrapInvalidHeartbeat(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 0,
	})
	err := svc.bootstrap()
	if !errors.Is(err, ErrInvalidHeartbeatInterval) {
		t.Fatalf("expected ErrInvalidHeartbeatInterval, got %v", err)
	}
}
