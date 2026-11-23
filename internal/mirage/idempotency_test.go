package mirage

import (
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Mirage server with one registered ghost and one single-command intent awaiting observation.
func newIdempotencyServer(t *testing.T) (*Server, session.Event) {
	t.Helper()
	srv := NewServer()
	srv.UpsertRegistration("127.0.0.1:10000", session.Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList:     []session.SeedInfo{{ID: "seed.flow", Name: "Flow", Description: "flow"}},
	})
	if err := srv.SubmitIssue(IssueEnv{
		IntentID:    "intent.idem.1",
		Actor:       "user:test",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "status",
		Stages: []IssueStage{{
			ID:       "stage.1",
			Barrier:  true,
			Commands: []IssueCommand{{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"}},
		}},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}
	event := session.Event{
		EventID:     "evt.cmd.intent.idem.1.1",
		CommandID:   "cmd.intent.idem.1.1",
		IntentID:    "intent.idem.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	return srv, event
}

func eventCount(t *testing.T, srv *Server, ghostID string) int {
	t.Helper()
	for _, g := range srv.SnapshotRegisteredGhosts() {
		if g.GhostID == ghostID {
			return int(g.EventCount)
		}
	}
	t.Fatalf("ghost %q not registered", ghostID)
	return 0
}

func observedEventCount(t *testing.T, srv *Server, intentID string) int {
	t.Helper()
	snap, ok := srv.SnapshotIntent(intentID)
	if !ok {
		t.Fatalf("intent %q missing", intentID)
	}
	return len(snap.Observed.Events)
}

func TestDuplicateEventDoesNotReMutateObservedState(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)

	first := srv.IngestEvent("ghost.alpha", event)
	if first.Duplicate || !first.Matched || first.ObserveErr != nil {
		t.Fatalf("unexpected first ingest: %+v", first)
	}
	second := srv.IngestEvent("ghost.alpha", event)
	if !second.Duplicate {
		t.Fatalf("expected duplicate=true on redelivery")
	}

	if got := len(srv.RecentReports(0)); got != 1 {
		t.Fatalf("report history grew on duplicate: %d", got)
	}
	if got := observedEventCount(t, srv, "intent.idem.1"); got != 1 {
		t.Fatalf("observed events grew on duplicate: %d", got)
	}
	if got := eventCount(t, srv, "ghost.alpha"); got != 1 {
		t.Fatalf("ghost event count grew on duplicate: %d", got)
	}
}

func TestDuplicateEventAckIsIdempotentByEventID(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)

	first := srv.IngestEvent("ghost.alpha", event)
	second := srv.IngestEvent("ghost.alpha", event)

	if second.Ack != first.Ack {
		t.Fatalf("event.ack differs on redelivery:\nfirst=%+v\nsecond=%+v", first.Ack, second.Ack)
	}
	if second.Ack.AckStatus != session.AckStatusAccepted {
		t.Fatalf("duplicate must be acked accepted, got %q", second.Ack.AckStatus)
	}
}

func TestEventForAlreadyObservedCommandDoesNotAppendReport(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)

	// First observation without an ack (e.g. the direct reconcile path), then session delivery.
	if _, _, dup, err := srv.ObserveEvent(event); err != nil || dup {
		t.Fatalf("first observe: dup=%v err=%v", dup, err)
	}
	ingest := srv.IngestEvent("ghost.alpha", event)
	if !ingest.Duplicate {
		t.Fatalf("expected duplicate observation for known command_id")
	}
	if ingest.Ack.AckStatus != session.AckStatusAccepted {
		t.Fatalf("event must still be acked, got %q", ingest.Ack.AckStatus)
	}
	if got := len(srv.RecentReports(0)); got != 1 {
		t.Fatalf("report history grew on duplicate observation: %d", got)
	}
}

func TestEventAckRetainedForDuplicateWindow(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)
	base := time.Now()
	srv.now = func() time.Time { return base }

	first := srv.IngestEvent("ghost.alpha", event)

	srv.now = func() time.Time { return base.Add(DuplicateWindow - time.Second) }
	inside := srv.IngestEvent("ghost.alpha", event)
	if !inside.Duplicate || inside.Ack != first.Ack {
		t.Fatalf("ack not retained inside window: %+v", inside)
	}

	// After expiry a fresh ack is issued, but observed state still dedupes by command_id.
	srv.now = func() time.Time { return base.Add(DuplicateWindow + time.Second) }
	after := srv.IngestEvent("ghost.alpha", event)
	if after.Ack.TimestampMS == first.Ack.TimestampMS {
		t.Fatalf("expected fresh ack after window expiry")
	}
	if got := len(srv.RecentReports(0)); got != 1 {
		t.Fatalf("report history grew after ack expiry: %d", got)
	}
	if got := observedEventCount(t, srv, "intent.idem.1"); got != 1 {
		t.Fatalf("observed events grew after ack expiry: %d", got)
	}
}
