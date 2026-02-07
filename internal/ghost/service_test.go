package ghost

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
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

func TestServiceBootstrapInvalidMiragePolicy(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: time.Second,
		Mirage: MirageSessionConfig{
			Policy:        MirageSessionPolicy("invalid"),
			SessionConfig: session.DefaultConfig(),
		},
	})
	err := svc.bootstrap()
	if !errors.Is(err, ErrInvalidMiragePolicy) {
		t.Fatalf("expected ErrInvalidMiragePolicy, got %v", err)
	}
}

func TestServiceServeHeadlessNoMirageSession(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 10 * time.Millisecond,
		Mirage: MirageSessionConfig{
			Policy:        MiragePolicyHeadless,
			SessionConfig: session.DefaultConfig(),
		},
	})
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	if err := svc.serve(ctx); err != nil {
		t.Fatalf("serve failed: %v", err)
	}
	if svc.MirageSession() != nil {
		t.Fatalf("expected no mirage session in headless mode")
	}
}

func TestServiceServeAutoConnectsMirage(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	mcfg := mirage.DefaultServiceConfig()
	mcfg.RequireIdentityBinding = true
	mcfg.Session.HandshakeTimeout = 2 * time.Second
	msvc := mirage.NewServiceWithConfig(mcfg)

	mctx, mcancel := context.WithCancel(context.Background())
	defer mcancel()
	mdone := make(chan error, 1)
	go func() {
		mdone <- msvc.Serve(mctx, ln)
	}()

	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 20 * time.Millisecond,
		Mirage: MirageSessionConfig{
			Policy:        MiragePolicyAuto,
			Address:       ln.Addr().String(),
			PeerIdentity:  "ghost.alpha",
			SessionConfig: session.DefaultConfig(),
		},
	})
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sctx, scancel := context.WithCancel(context.Background())
	sdone := make(chan error, 1)
	go func() {
		sdone <- svc.serve(sctx)
	}()

	if !waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		return len(msvc.SnapshotRegisteredGhosts()) == 1
	}) {
		scancel()
		mcancel()
		_ = <-sdone
		_ = <-mdone
		t.Fatalf("ghost did not register with mirage")
	}

	if svc.MirageSession() == nil {
		scancel()
		mcancel()
		_ = <-sdone
		_ = <-mdone
		t.Fatalf("expected active mirage session")
	}

	scancel()
	if err := <-sdone; err != nil {
		t.Fatalf("ghost serve exit err: %v", err)
	}
	mcancel()
	if err := <-mdone; err != nil {
		t.Fatalf("mirage serve exit err: %v", err)
	}
}

func waitForCondition(timeout time.Duration, interval time.Duration, fn func() bool) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return true
		}
		time.Sleep(interval)
	}
	return fn()
}
