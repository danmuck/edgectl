package mirage

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	"github.com/danmuck/edgectl/internal/testutil/tlstest"
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

func TestServiceEventAckReplayAcrossReconnect(t *testing.T) {
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

	clientCfg := ghost.MirageClientConfig{
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
	}
	client, err := ghost.NewMirageClient(clientCfg)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}

	connectCtxA, connectCancelA := context.WithTimeout(ctx, 3*time.Second)
	defer connectCancelA()
	gsA, err := client.ConnectAndRegister(connectCtxA)
	if err != nil {
		t.Fatalf("connect and register (A): %v", err)
	}

	event := ghost.EventEnv{
		EventID:     "evt.reconnect.1",
		CommandID:   "cmd.reconnect.1",
		IntentID:    "intent.reconnect.1",
		GhostID:     "ghost.alpha",
		SeedID:      "seed.flow",
		Outcome:     ghost.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ackA, err := gsA.SendEventWithAck(connectCtxA, event)
	if err != nil {
		_ = gsA.Close()
		t.Fatalf("send event (A): %v", err)
	}
	if err := gsA.Close(); err != nil {
		t.Fatalf("close session (A): %v", err)
	}

	if !waitForGhostState(2*time.Second, 20*time.Millisecond, svc, "ghost.alpha", func(g RegisteredGhost) bool {
		return !g.Connected && g.EventCount == 1
	}) {
		t.Fatalf("ghost state did not transition to disconnected with preserved event count")
	}

	connectCtxB, connectCancelB := context.WithTimeout(ctx, 3*time.Second)
	defer connectCancelB()
	gsB, err := client.ConnectAndRegister(connectCtxB)
	if err != nil {
		t.Fatalf("connect and register (B): %v", err)
	}
	defer gsB.Close()

	ackB, err := gsB.SendEventWithAck(connectCtxB, event)
	if err != nil {
		t.Fatalf("send event replay (B): %v", err)
	}
	if ackA.AckStatus != ackB.AckStatus {
		t.Fatalf("ack status mismatch across reconnect: a=%q b=%q", ackA.AckStatus, ackB.AckStatus)
	}
	if ackA.AckCode != ackB.AckCode {
		t.Fatalf("ack code mismatch across reconnect: a=%d b=%d", ackA.AckCode, ackB.AckCode)
	}
	if ackA.TimestampMS != ackB.TimestampMS {
		t.Fatalf("ack timestamp mismatch across reconnect: a=%d b=%d", ackA.TimestampMS, ackB.TimestampMS)
	}

	if !waitForGhostState(2*time.Second, 20*time.Millisecond, svc, "ghost.alpha", func(g RegisteredGhost) bool {
		return g.Connected && g.EventCount == 1
	}) {
		t.Fatalf("ghost state did not preserve idempotent event count after replay")
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}

func waitForGhostState(
	timeout time.Duration,
	interval time.Duration,
	svc *Service,
	ghostID string,
	match func(RegisteredGhost) bool,
) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, g := range svc.SnapshotRegisteredGhosts() {
			if g.GhostID == ghostID && match(g) {
				return true
			}
		}
		time.Sleep(interval)
	}
	for _, g := range svc.SnapshotRegisteredGhosts() {
		if g.GhostID == ghostID && match(g) {
			return true
		}
	}
	return false
}

func TestServiceRegistrationTLSMTLSIdentityBound(t *testing.T) {
	testlog.Start(t)

	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	serverCert, serverKey := ca.IssueServerCert(t, dir, "mirage.local", []string{"mirage.local"}, []net.IP{net.ParseIP("127.0.0.1")})
	clientCert, clientKey := ca.IssueClientCert(t, dir, "ghost.alpha")

	cfg := DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.SecurityMode = session.SecurityModeProduction
	cfg.Session.TLS.Enabled = true
	cfg.Session.TLS.Mutual = true
	cfg.Session.TLS.CertFile = serverCert
	cfg.Session.TLS.KeyFile = serverKey
	cfg.Session.TLS.CAFile = ca.CAFile()
	cfg.Session.HandshakeTimeout = 2 * time.Second
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second

	svc := NewServiceWithConfig(cfg)
	tlsCfg, err := svc.serverTLSConfig()
	if err != nil {
		t.Fatalf("server tls config: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.Serve(ctx, ln)
	}()

	clientSessionCfg := session.DefaultConfig()
	clientSessionCfg.SecurityMode = session.SecurityModeProduction
	clientSessionCfg.TLS.Enabled = true
	clientSessionCfg.TLS.Mutual = true
	clientSessionCfg.TLS.CertFile = clientCert
	clientSessionCfg.TLS.KeyFile = clientKey
	clientSessionCfg.TLS.CAFile = ca.CAFile()
	clientSessionCfg.TLS.ServerName = "mirage.local"

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:      ln.Addr().String(),
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{
			{ID: "seed.flow", Name: "Flow", Description: "Deterministic control-flow seed"},
		},
		Session: clientSessionCfg,
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

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}

func TestServiceRegistrationTLSIdentityMismatchRejected(t *testing.T) {
	testlog.Start(t)

	dir := t.TempDir()
	ca := tlstest.NewAuthority(t, dir, "edgectl-test-ca")
	serverCert, serverKey := ca.IssueServerCert(t, dir, "mirage.local", []string{"mirage.local"}, []net.IP{net.ParseIP("127.0.0.1")})
	clientCert, clientKey := ca.IssueClientCert(t, dir, "ghost.beta")

	cfg := DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.SecurityMode = session.SecurityModeProduction
	cfg.Session.TLS.Enabled = true
	cfg.Session.TLS.Mutual = true
	cfg.Session.TLS.CertFile = serverCert
	cfg.Session.TLS.KeyFile = serverKey
	cfg.Session.TLS.CAFile = ca.CAFile()
	cfg.Session.HandshakeTimeout = 2 * time.Second
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second

	svc := NewServiceWithConfig(cfg)
	tlsCfg, err := svc.serverTLSConfig()
	if err != nil {
		t.Fatalf("server tls config: %v", err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatalf("listen tls: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.Serve(ctx, ln)
	}()

	clientSessionCfg := session.DefaultConfig()
	clientSessionCfg.SecurityMode = session.SecurityModeProduction
	clientSessionCfg.TLS.Enabled = true
	clientSessionCfg.TLS.Mutual = true
	clientSessionCfg.TLS.CertFile = clientCert
	clientSessionCfg.TLS.KeyFile = clientKey
	clientSessionCfg.TLS.CAFile = ca.CAFile()
	clientSessionCfg.TLS.ServerName = "mirage.local"

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:            ln.Addr().String(),
		GhostID:            "ghost.alpha",
		PeerIdentity:       "ghost.alpha",
		SeedList:           []session.SeedInfo{},
		Session:            clientSessionCfg,
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
