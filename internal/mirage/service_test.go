package mirage

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestServiceRegistrationAndEventAck(t *testing.T) {
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
	go func() {
		done <- svc.Serve(ctx, ln)
	}()

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:      ln.Addr().String(),
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{
			{ID: "seed.flow", Name: "Flow", Description: "Deterministic control-flow seed"},
		},
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

	connectCtx, connectCancel := context.WithTimeout(ctx, 3*time.Second)
	defer connectCancel()
	gs, err := client.ConnectAndRegister(connectCtx)
	if err != nil {
		t.Fatalf("connect and register: %v", err)
	}
	defer gs.Close()

	event := ghost.EventEnv{
		EventID:     "evt.1",
		CommandID:   "cmd.1",
		IntentID:    "intent.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     ghost.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ackA, err := gs.SendEventWithAck(connectCtx, event)
	if err != nil {
		t.Fatalf("send event (first): %v", err)
	}
	if ackA.AckStatus != session.AckStatusAccepted {
		t.Fatalf("unexpected ack status: %+v", ackA)
	}
	ackB, err := gs.SendEventWithAck(connectCtx, event)
	if err != nil {
		t.Fatalf("send event (duplicate): %v", err)
	}
	if ackA.TimestampMS != ackB.TimestampMS {
		t.Fatalf("expected idempotent ack timestamp, got a=%d b=%d", ackA.TimestampMS, ackB.TimestampMS)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}

func TestServiceRegistrationIdentityMismatchRejected(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg := DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.HandshakeTimeout = 2 * time.Second
	svc := NewServiceWithConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.Serve(ctx, ln)
	}()

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:            ln.Addr().String(),
		GhostID:            "ghost.alpha",
		PeerIdentity:       "peer.other",
		SeedList:           []session.SeedInfo{},
		Session:            session.DefaultConfig(),
		MaxConnectAttempts: 1,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	connectCtx, connectCancel := context.WithTimeout(ctx, 3*time.Second)
	defer connectCancel()
	if _, err := client.ConnectAndRegister(connectCtx); !errors.Is(err, ghost.ErrRegistrationRejected) {
		t.Fatalf("expected ErrRegistrationRejected, got %v", err)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}
