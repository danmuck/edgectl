package mirage

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestServerLifecycleAndStatus(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	if err := srv.Appear(MirageConfig{MirageID: "mirage.alpha"}); err != nil {
		t.Fatalf("appear: %v", err)
	}
	if err := srv.Shimmer(); err != nil {
		t.Fatalf("shimmer: %v", err)
	}
	if err := srv.Seed(); err != nil {
		t.Fatalf("seed: %v", err)
	}

	status := srv.Status()
	if status.MirageID != "mirage.alpha" {
		t.Fatalf("unexpected mirage id: %q", status.MirageID)
	}
	if status.Phase != PhaseSeeded {
		t.Fatalf("unexpected phase: %q", status.Phase)
	}
}

func TestServerRegistrationAndEventAckIdempotent(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	reg := session.Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{
			{ID: "seed.flow", Name: "Flow", Description: "flow"},
		},
	}
	ack := srv.UpsertRegistration("127.0.0.1:10000", reg)
	if ack.Status != session.AckStatusAccepted {
		t.Fatalf("unexpected register ack: %+v", ack)
	}

	event := session.Event{
		EventID:     "evt.1",
		CommandID:   "cmd.1",
		IntentID:    "intent.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ackA := srv.AcceptEvent("ghost.alpha", event)
	ackB := srv.AcceptEvent("ghost.alpha", event)
	if ackA.TimestampMS != ackB.TimestampMS {
		t.Fatalf("expected idempotent event ack timestamp, a=%d b=%d", ackA.TimestampMS, ackB.TimestampMS)
	}
}

func TestServerRegistrationEnrichesMongodDescriptionWithEndpointURI(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	reg := session.Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{
			{ID: "seed.mongod", Name: "MongoDB (mongod)", Description: "Predefined mongod service adapter for Linux hosts"},
			{ID: "seed.flow", Name: "Flow", Description: "flow"},
			{ID: "seed.kv", Name: "KV", Description: "Temporary key-value state storage seed for control-plane persistence"},
			{ID: "seed.fs", Name: "Filesystem", Description: "Temporary file persistence seed scoped to local/dir"},
		},
	}
	_ = srv.UpsertRegistration("10.42.0.5:9000", reg)

	snapshot := srv.SnapshotRegisteredGhosts()
	if len(snapshot) != 1 {
		t.Fatalf("unexpected registry count: %d", len(snapshot))
	}
	if len(snapshot[0].SeedList) != 4 {
		t.Fatalf("unexpected seed count: %d", len(snapshot[0].SeedList))
	}

	var mongodDesc string
	var flowDesc string
	var kvDesc string
	var fsDesc string
	for _, seed := range snapshot[0].SeedList {
		if seed.ID == "seed.mongod" {
			mongodDesc = seed.Description
		}
		if seed.ID == "seed.flow" {
			flowDesc = seed.Description
		}
		if seed.ID == "seed.kv" {
			kvDesc = seed.Description
		}
		if seed.ID == "seed.fs" {
			fsDesc = seed.Description
		}
	}
	if !strings.Contains(mongodDesc, "endpoint_uri=mongodb://10.42.0.5:27017/?directConnection=true") {
		t.Fatalf("unexpected mongod description: %q", mongodDesc)
	}
	if !strings.Contains(mongodDesc, "host=10.42.0.5") || !strings.Contains(mongodDesc, "ghost_id=ghost.alpha") {
		t.Fatalf("expected mongod host/ghost enrichment, got %q", mongodDesc)
	}
	if !strings.Contains(flowDesc, "seed_scope=control_plane") || !strings.Contains(flowDesc, "dispatch=ghost_execute") || !strings.Contains(flowDesc, "ghost_id=ghost.alpha") {
		t.Fatalf("unexpected flow description enrichment: %q", flowDesc)
	}
	if !strings.Contains(kvDesc, "persistence=in_memory") || !strings.Contains(kvDesc, "seed_scope=ghost_local_cache") || !strings.Contains(kvDesc, "ghost_id=ghost.alpha") {
		t.Fatalf("unexpected kv description enrichment: %q", kvDesc)
	}
	if !strings.Contains(fsDesc, "path_root=local/dir/ghost.alpha") || !strings.Contains(fsDesc, "seed_scope=ghost_local_filesystem") || !strings.Contains(fsDesc, "ghost_id=ghost.alpha") {
		t.Fatalf("unexpected fs description enrichment: %q", fsDesc)
	}
}

func TestServerDelegatesOrchestration(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	exec := &fakeExecutor{}
	if err := srv.RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	if err := srv.SubmitIssue(IssueEnv{
		IntentID:     "intent.1",
		Actor:        "user:dan",
		TargetScope:  "ghost:ghost.alpha",
		Objective:    "status",
		Operation:    "status",
		SeedSelector: "seed.flow",
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}

	report, err := srv.ReconcileIntent(context.Background(), "intent.1")
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if report.Phase != ReportPhaseComplete {
		t.Fatalf("unexpected report phase: %q", report.Phase)
	}
	reports := srv.RecentReports(10)
	if len(reports) != 1 {
		t.Fatalf("expected 1 emitted report, got %d", len(reports))
	}
}

func TestServerLifecycleOrderInvalid(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	if err := srv.Shimmer(); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder, got %v", err)
	}
}

type fakeSpawner struct {
	out SpawnGhostResult
	err error
}

func (s fakeSpawner) SpawnLocalGhost(_ context.Context, _ SpawnGhostRequest) (SpawnGhostResult, error) {
	if s.err != nil {
		return SpawnGhostResult{}, s.err
	}
	return s.out, nil
}

func TestServerSpawnLocalGhostRequiresSpawner(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	_, err := srv.SpawnLocalGhost(context.Background(), SpawnGhostRequest{
		TargetName: "edge-1",
		AdminAddr:  "127.0.0.1:7119",
	})
	if !errors.Is(err, ErrNoGhostSpawner) {
		t.Fatalf("expected ErrNoGhostSpawner, got %v", err)
	}
}

func TestServerSpawnLocalGhostViaSpawner(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	srv.SetGhostSpawner(fakeSpawner{
		out: SpawnGhostResult{
			TargetName: "ghost.local.edge.1",
			GhostID:    "ghost.local.edge.1",
			AdminAddr:  "127.0.0.1:7119",
		},
	})
	out, err := srv.SpawnLocalGhost(context.Background(), SpawnGhostRequest{
		TargetName: "edge-1",
		AdminAddr:  "127.0.0.1:7119",
	})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	if out.GhostID != "ghost.local.edge.1" {
		t.Fatalf("unexpected ghost id: %q", out.GhostID)
	}
}
