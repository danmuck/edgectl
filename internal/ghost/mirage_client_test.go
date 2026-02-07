package ghost

import (
	"bufio"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestMirageSessionSendEventWithAckTimeout(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	done := make(chan error, 1)
	go func() {
		done <- serveNoAckEndpoint(ln)
	}()

	cfg := session.DefaultConfig()
	cfg.ConnectTimeout = 500 * time.Millisecond
	cfg.HandshakeTimeout = 500 * time.Millisecond
	cfg.ReadTimeout = 40 * time.Millisecond
	cfg.WriteTimeout = 200 * time.Millisecond
	cfg.AckTimeout = 220 * time.Millisecond
	cfg.Backoff.InitialDelay = 10 * time.Millisecond
	cfg.Backoff.Multiplier = 1.5
	cfg.Backoff.MaxDelay = 20 * time.Millisecond
	cfg.Backoff.Jitter = false

	client, err := NewMirageClient(MirageClientConfig{
		Address:            ln.Addr().String(),
		GhostID:            "ghost.alpha",
		PeerIdentity:       "ghost.alpha",
		SeedList:           []session.SeedInfo{{ID: "seed.flow", Name: "Flow", Description: "Deterministic control-flow seed"}},
		Session:            cfg,
		MaxConnectAttempts: 1,
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	gs, err := client.ConnectAndRegister(ctx)
	if err != nil {
		_ = ln.Close()
		_ = <-done
		t.Fatalf("connect and register: %v", err)
	}
	defer gs.Close()

	_, err = gs.SendEventWithAck(ctx, EventEnv{
		EventID:     "evt.timeout.1",
		CommandID:   "cmd.timeout.1",
		IntentID:    "intent.timeout.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	})
	if !errors.Is(err, ErrAckTimeout) {
		_ = ln.Close()
		_ = <-done
		t.Fatalf("expected ErrAckTimeout, got %v", err)
	}

	if err := gs.Close(); err != nil {
		_ = ln.Close()
		_ = <-done
		t.Fatalf("close session: %v", err)
	}
	if err := ln.Close(); err != nil {
		_ = <-done
		t.Fatalf("close listener: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("no-ack endpoint exit err: %v", err)
	}
}

func serveNoAckEndpoint(ln net.Listener) error {
	defer ln.Close()

	conn, err := ln.Accept()
	if err != nil {
		if errors.Is(err, net.ErrClosed) {
			return nil
		}
		return err
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	reg, err := session.ReadRegistration(reader)
	if err != nil {
		return err
	}
	if err := session.WriteRegistrationAck(conn, session.RegistrationAck{
		Status:      session.AckStatusAccepted,
		Code:        0,
		Message:     "registered",
		GhostID:     reg.GhostID,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}); err != nil {
		return err
	}

	for {
		if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
			return err
		}
		if _, err := session.ReadFrame(reader, frame.DefaultLimits()); err != nil {
			var netErr net.Error
			if errors.Is(err, net.ErrClosed) || errors.Is(err, context.Canceled) {
				return nil
			}
			if errors.As(err, &netErr) && netErr.Timeout() {
				continue
			}
			return nil
		}
	}
}
