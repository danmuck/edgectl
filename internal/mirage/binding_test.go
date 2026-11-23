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
