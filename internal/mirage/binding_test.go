package mirage

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func assertNothingObserved(t *testing.T, srv *Server) {
	t.Helper()
	if got := len(srv.RecentReports(0)); got != 0 {
		t.Fatalf("rejected event produced %d reports", got)
	}
	if got := observedEventCount(t, srv, "intent.idem.1"); got != 0 {
		t.Fatalf("rejected event was observed: %d events", got)
	}
}

func TestEventWithForeignDeclaredGhostIDIsRejected(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)
	srv.UpsertRegistration("127.0.0.1:10001", session.Registration{GhostID: "ghost.beta", PeerIdentity: "ghost.beta"})

	// Session is bound to ghost.beta but the event claims ghost.alpha.
	ingest := srv.IngestEvent("ghost.beta", event)
	if !ingest.Rejected {
		t.Fatalf("expected rejection, got %+v", ingest)
	}
	if ingest.Ack.AckStatus != session.AckStatusRejected || ingest.Ack.AckCode != schema.ErrCodeTransportFailure {
		t.Fatalf("unexpected reject ack: %+v", ingest.Ack)
	}
	assertNothingObserved(t, srv)
	if got := eventCount(t, srv, "ghost.beta"); got != 0 {
		t.Fatalf("rejected event counted for session ghost: %d", got)
	}
}

func TestEventForCommandPlannedOnAnotherGhostIsRejected(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)
	srv.UpsertRegistration("127.0.0.1:10001", session.Registration{GhostID: "ghost.beta", PeerIdentity: "ghost.beta"})

	// Declared ghost_id matches the session, but the command was planned for ghost.alpha.
	spoofed := event
	spoofed.GhostID = "ghost.beta"
	spoofed.EventID = "evt.spoofed"
	spoofed.Outcome = OutcomeError
	ingest := srv.IngestEvent("ghost.beta", spoofed)
	if !ingest.Rejected {
		t.Fatalf("expected rejection, got %+v", ingest)
	}
	assertNothingObserved(t, srv)

	// The real ghost's result must still be accepted afterwards, not deduped away.
	real := srv.IngestEvent("ghost.alpha", event)
	if real.Rejected || real.Duplicate || !real.Matched {
		t.Fatalf("legitimate event not accepted after spoof: %+v", real)
	}
	snap, _ := srv.SnapshotIntent("intent.idem.1")
	if got := snap.Observed.ByCommandID[event.CommandID].Outcome; got != OutcomeSuccess {
		t.Fatalf("observed outcome = %q, want the legitimate %q", got, OutcomeSuccess)
	}
}

func TestObserveRejectsCommandPlannedForAnotherGhostWithoutMutation(t *testing.T) {
	testlog.Start(t)
	srv, event := newIdempotencyServer(t)

	event.GhostID = "ghost.beta"
	_, _, _, err := srv.ObserveEvent("ghost.beta", event)
	if !errors.Is(err, ErrCommandGhostMismatch) {
		t.Fatalf("expected ErrCommandGhostMismatch, got %v", err)
	}
	assertNothingObserved(t, srv)
}

func TestSubmitIssueRejectsCommandIDCollisionAcrossIntents(t *testing.T) {
	testlog.Start(t)
	srv := NewServer()
	issue := func(intentID, ghostID string) IssueEnv {
		return IssueEnv{
			IntentID:    intentID,
			Actor:       "user:test",
			TargetScope: "ghost:" + ghostID,
			Objective:   "status",
			Stages: []IssueStage{{
				ID:       "stage.1",
				Commands: []IssueCommand{{GhostID: ghostID, SeedSelector: "seed.flow", Operation: "status"}},
			}},
		}
	}
	if err := srv.SubmitIssue(issue("x/y", "ghost.alpha")); err != nil {
		t.Fatalf("first issue: %v", err)
	}
	// "x.y" sanitizes to the same command_id prefix as "x/y".
	err := srv.SubmitIssue(issue("x.y", "ghost.beta"))
	if !errors.Is(err, ErrInvalidIssue) {
		t.Fatalf("expected ErrInvalidIssue for colliding command_id, got %v", err)
	}
	if _, ok := srv.SnapshotIntent("x.y"); ok {
		t.Fatalf("colliding intent must not be stored")
	}
	// Resubmitting the same intent_id replaces its own plan and is allowed.
	if err := srv.SubmitIssue(issue("x/y", "ghost.alpha")); err != nil {
		t.Fatalf("resubmit same intent: %v", err)
	}
}

func TestEventForUnknownCommandWithMatchingGhostIsAccepted(t *testing.T) {
	testlog.Start(t)
	srv, _ := newIdempotencyServer(t)

	ingest := srv.IngestEvent("ghost.alpha", session.Event{
		EventID:     "evt.unplanned",
		CommandID:   "cmd.unplanned",
		IntentID:    "intent.unplanned",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	})
	if ingest.Rejected || ingest.Ack.AckStatus != session.AckStatusAccepted {
		t.Fatalf("unplanned event from bound ghost should be acked: %+v", ingest)
	}
}

func TestServiceClosesSessionOnIdentityBindingMismatch(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg := DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second
	cfg.Session.HandshakeTimeout = 2 * time.Second
	svc := NewServiceWithConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Serve(ctx, ln) }()

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:      ln.Addr().String(),
		GhostID:      "ghost.beta",
		PeerIdentity: "ghost.beta",
		SeedList:     []session.SeedInfo{{ID: "seed.flow", Name: "Flow", Description: "flow"}},
		Session: session.Config{
			ConnectTimeout:   2 * time.Second,
			HandshakeTimeout: 2 * time.Second,
			ReadTimeout:      2 * time.Second,
			WriteTimeout:     2 * time.Second,
			AckTimeout:       3 * time.Second,
			Backoff:          session.DefaultConfig().Backoff,
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	opCtx, opCancel := context.WithTimeout(ctx, 3*time.Second)
	defer opCancel()
	gs, err := client.ConnectAndRegister(opCtx)
	if err != nil {
		t.Fatalf("connect and register: %v", err)
	}
	defer gs.Close()

	spoofed := ghost.EventEnv{
		EventID:     "evt.spoof",
		CommandID:   "cmd.spoof",
		IntentID:    "intent.spoof",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     ghost.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ack, err := gs.SendEventWithAck(opCtx, spoofed)
	if !errors.Is(err, ghost.ErrAckRejected) {
		t.Fatalf("expected ErrAckRejected, got ack=%+v err=%v", ack, err)
	}
	if ack.AckCode != schema.ErrCodeTransportFailure {
		t.Fatalf("expected ack_code=%d, got %d", schema.ErrCodeTransportFailure, ack.AckCode)
	}
	// reliability.toml ack_reject_behavior: Ghost marks the event delivery_failed.
	if outbox := gs.OutboxSnapshot(); len(outbox) != 1 || outbox[0].State != session.DeliveryFailed {
		t.Fatalf("expected one delivery_failed outbox entry, got %+v", outbox)
	}

	// Session must be closed after the rejected ack; a follow-up event cannot be acked.
	followUp := spoofed
	followUp.EventID = "evt.after"
	followUp.GhostID = "ghost.beta"
	if _, err := gs.SendEventWithAck(opCtx, followUp); err == nil {
		t.Fatalf("expected session closed after binding mismatch")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}
