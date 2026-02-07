package mirage

import (
	"context"
	"errors"
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
}

func TestServerLifecycleOrderInvalid(t *testing.T) {
	testlog.Start(t)

	srv := NewServer()
	if err := srv.Shimmer(); !errors.Is(err, ErrLifecycleOrder) {
		t.Fatalf("expected ErrLifecycleOrder, got %v", err)
	}
}
