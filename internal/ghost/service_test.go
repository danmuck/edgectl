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

func TestServiceServeAutoReconnectsAfterMirageRestart(t *testing.T) {
	testlog.Start(t)

	lnA, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen a: %v", err)
	}
	addr := lnA.Addr().String()

	mcfg := mirage.DefaultServiceConfig()
	mcfg.RequireIdentityBinding = true
	mcfg.Session.HandshakeTimeout = 500 * time.Millisecond
	mcfg.Session.ReadTimeout = 500 * time.Millisecond
	mcfg.Session.WriteTimeout = 500 * time.Millisecond

	msvcA := mirage.NewServiceWithConfig(mcfg)
	mctxA, mcancelA := context.WithCancel(context.Background())
	mdoneA := make(chan error, 1)
	go func() {
		mdoneA <- msvcA.Serve(mctxA, lnA)
	}()

	scfg := DefaultServiceConfig()
	scfg.GhostID = "ghost.alpha"
	scfg.BuiltinSeedIDs = []string{"seed.flow"}
	scfg.HeartbeatInterval = 25 * time.Millisecond
	scfg.Mirage.Policy = MiragePolicyAuto
	scfg.Mirage.Address = addr
	scfg.Mirage.PeerIdentity = "ghost.alpha"
	scfg.Mirage.SessionConfig.ConnectTimeout = 200 * time.Millisecond
	scfg.Mirage.SessionConfig.HandshakeTimeout = 500 * time.Millisecond
	scfg.Mirage.SessionConfig.ReadTimeout = 250 * time.Millisecond
	scfg.Mirage.SessionConfig.WriteTimeout = 250 * time.Millisecond
	scfg.Mirage.SessionConfig.AckTimeout = 400 * time.Millisecond
	scfg.Mirage.SessionConfig.HeartbeatInterval = 50 * time.Millisecond
	scfg.Mirage.SessionConfig.SessionDeadAfter = 500 * time.Millisecond
	scfg.Mirage.SessionConfig.Backoff.InitialDelay = 50 * time.Millisecond
	scfg.Mirage.SessionConfig.Backoff.MaxDelay = 200 * time.Millisecond

	svc := NewServiceWithConfig(scfg)
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	sctx, scancel := context.WithCancel(context.Background())
	sdone := make(chan error, 1)
	go func() {
		sdone <- svc.serve(sctx)
	}()

	if !waitForCondition(2*time.Second, 20*time.Millisecond, func() bool {
		return len(msvcA.SnapshotRegisteredGhosts()) == 1
	}) {
		scancel()
		mcancelA()
		_ = <-sdone
		_ = <-mdoneA
		t.Fatalf("ghost did not register with first mirage")
	}

	mcancelA()
	if err := <-mdoneA; err != nil {
		scancel()
		_ = <-sdone
		t.Fatalf("mirage A exit err: %v", err)
	}

	lnB, err := net.Listen("tcp", addr)
	if err != nil {
		scancel()
		_ = <-sdone
		t.Fatalf("listen b: %v", err)
	}
	msvcB := mirage.NewServiceWithConfig(mcfg)
	mctxB, mcancelB := context.WithCancel(context.Background())
	defer mcancelB()
	mdoneB := make(chan error, 1)
	go func() {
		mdoneB <- msvcB.Serve(mctxB, lnB)
	}()

	if !waitForCondition(4*time.Second, 25*time.Millisecond, func() bool {
		return len(msvcB.SnapshotRegisteredGhosts()) == 1
	}) {
		scancel()
		mcancelB()
		_ = <-sdone
		_ = <-mdoneB
		t.Fatalf("ghost did not reconnect/register after mirage restart")
	}

	scancel()
	if err := <-sdone; err != nil {
		t.Fatalf("ghost serve exit err: %v", err)
	}
	mcancelB()
	if err := <-mdoneB; err != nil {
		t.Fatalf("mirage B exit err: %v", err)
	}
}

func TestServiceSpawnManagedGhost(t *testing.T) {
	testlog.Start(t)

	root := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.local",
		BuiltinSeedIDs:    []string{"seed.flow", "seed.mongod"},
		HeartbeatInterval: time.Second,
		AdminListenAddr:   "127.0.0.1:7118",
		EnableClusterHost: true,
		Mirage: MirageSessionConfig{
			Policy:        MiragePolicyHeadless,
			SessionConfig: session.DefaultConfig(),
		},
	})
	if err := root.bootstrap(); err != nil {
		t.Fatalf("bootstrap root: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- root.serve(ctx)
	}()
	defer func() {
		cancel()
		_ = <-done
	}()

	out, err := root.SpawnManagedGhost(SpawnGhostRequest{
		TargetName: "edge-1",
		AdminAddr:  "127.0.0.1:7119",
	})
	if err != nil {
		t.Fatalf("spawn managed ghost: %v", err)
	}
	if out.GhostID != "ghost.local.edge.1" {
		t.Fatalf("unexpected ghost id: %q", out.GhostID)
	}
	if out.AdminAddr != "127.0.0.1:7119" {
		t.Fatalf("unexpected admin addr: %q", out.AdminAddr)
	}

	ok := waitForCondition(2*time.Second, 50*time.Millisecond, func() bool {
		conn, err := net.DialTimeout("tcp", out.AdminAddr, 200*time.Millisecond)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	})
	if !ok {
		t.Fatalf("spawned ghost admin endpoint not reachable: %s", out.AdminAddr)
	}
}

func TestServiceServeRequiredFailsWithoutMirage(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(ServiceConfig{
		GhostID:           "ghost.alpha",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 20 * time.Millisecond,
		Mirage: MirageSessionConfig{
			Policy:             MiragePolicyRequired,
			Address:            "127.0.0.1:1",
			PeerIdentity:       "ghost.alpha",
			MaxConnectAttempts: 1,
			SessionConfig:      session.DefaultConfig(),
		},
	})
	if err := svc.bootstrap(); err != nil {
		t.Fatalf("bootstrap failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := svc.serve(ctx)
	if err == nil {
		t.Fatalf("expected required mirage connection error")
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
