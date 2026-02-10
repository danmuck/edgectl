package session

import (
	"bufio"
	"bytes"
	"errors"
	"math/rand"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	logs "github.com/danmuck/smplog"
)

func TestNextBackoffDelayDeterministicNoJitter(t *testing.T) {
	testlog.Start(t)
	cfg := BackoffConfig{
		InitialDelay: 250 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		Jitter:       false,
	}
	if got := NextBackoffDelay(cfg, 1, nil); got != 250*time.Millisecond {
		t.Fatalf("attempt1 got=%v", got)
	}
	if got := NextBackoffDelay(cfg, 2, nil); got != 500*time.Millisecond {
		t.Fatalf("attempt2 got=%v", got)
	}
	if got := NextBackoffDelay(cfg, 3, nil); got != time.Second {
		t.Fatalf("attempt3 got=%v", got)
	}
	if got := NextBackoffDelay(cfg, 6, nil); got != 5*time.Second {
		t.Fatalf("attempt6 got=%v", got)
	}
}

func TestEventOutboxLifecycle(t *testing.T) {
	testlog.Start(t)
	o := NewEventOutbox()
	now := time.Unix(1700000000, 0)
	o.Upsert(PendingEvent{
		EventID:       "evt.1",
		CommandID:     "cmd.1",
		GhostID:       "ghost.a",
		QueuedAt:      now,
		AckDeadlineAt: now.Add(20 * time.Second),
	})
	item, ok := o.MarkAttempt("evt.1", now.Add(time.Second), "timeout")
	if !ok {
		t.Fatalf("missing pending item")
	}
	if item.Attempts != 1 {
		t.Fatalf("unexpected attempts=%d", item.Attempts)
	}
	if item.LastError != "timeout" {
		t.Fatalf("unexpected last error=%q", item.LastError)
	}
	if _, ok := o.Get("evt.1"); !ok {
		t.Fatalf("expected pending event")
	}
	o.Remove("evt.1")
	if _, ok := o.Get("evt.1"); ok {
		t.Fatalf("event should be removed")
	}
}

func TestRegistrationRoundTrip(t *testing.T) {
	testlog.Start(t)
	reg := Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList: []SeedInfo{
			{ID: "seed.flow", Name: "Flow", Description: "Deterministic control-flow seed"},
		},
	}
	var buf bytes.Buffer
	if err := WriteRegistration(&buf, reg); err != nil {
		t.Fatalf("write registration: %v", err)
	}
	got, err := ReadRegistration(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read registration: %v", err)
	}
	if got.GhostID != reg.GhostID || len(got.SeedList) != 1 || got.SeedList[0].ID != "seed.flow" {
		t.Fatalf("unexpected registration: %+v", got)
	}
}

func TestRegistrationAckRoundTrip(t *testing.T) {
	testlog.Start(t)
	ack := RegistrationAck{
		Status:      AckStatusAccepted,
		Code:        0,
		Message:     "ok",
		GhostID:     "ghost.alpha",
		TimestampMS: 1700000000000,
	}
	var buf bytes.Buffer
	if err := WriteRegistrationAck(&buf, ack); err != nil {
		t.Fatalf("write ack: %v", err)
	}
	got, err := ReadRegistrationAck(bufio.NewReader(&buf))
	if err != nil {
		t.Fatalf("read ack: %v", err)
	}
	if got.Status != AckStatusAccepted || got.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected ack: %+v", got)
	}
}

func TestEncodeDecodeEventFrame(t *testing.T) {
	testlog.Start(t)
	payload, err := EncodeEventFrame(42, Event{
		EventID:   "evt.42",
		CommandID: "cmd.42",
		IntentID:  "intent.42",
		GhostID:   "ghost.alpha",
		SeedID:    "seed.flow",
		Outcome:   "success",
	})
	if err != nil {
		t.Fatalf("encode event frame: %v", err)
	}

	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	got, err := DecodeEventFrame(fr)
	if err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if got.EventID != "evt.42" || got.CommandID != "cmd.42" || got.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected event: %+v", got)
	}
}

func TestEncodeDecodeCommandFrameWithAuth(t *testing.T) {
	testlog.Start(t)
	payload, err := EncodeCommandFrameWithAuth(77, Command{
		CommandID:    "cmd.77",
		IntentID:     "intent.77",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	}, []byte("token-77"))
	if err != nil {
		t.Fatalf("encode command frame: %v", err)
	}

	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if string(fr.Auth) != "token-77" {
		t.Fatalf("unexpected auth block: %q", string(fr.Auth))
	}
	got, err := DecodeCommandFrame(fr)
	if err != nil {
		t.Fatalf("decode command: %v", err)
	}
	if got.CommandID != "cmd.77" || got.IntentID != "intent.77" || got.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected command: %+v", got)
	}
}

func TestEncodeDecodeEventAckFrame(t *testing.T) {
	testlog.Start(t)
	payload, err := EncodeEventAckFrame(99, EventAck{
		EventID:     "evt.99",
		CommandID:   "cmd.99",
		GhostID:     "ghost.alpha",
		AckStatus:   AckStatusAccepted,
		AckCode:     0,
		TimestampMS: 1700000000123,
	})
	if err != nil {
		t.Fatalf("encode event.ack frame: %v", err)
	}

	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	got, err := DecodeEventAckFrame(fr)
	if err != nil {
		t.Fatalf("decode event.ack: %v", err)
	}
	if got.EventID != "evt.99" || got.AckStatus != AckStatusAccepted || got.TimestampMS == 0 {
		t.Fatalf("unexpected event.ack: %+v", got)
	}
}

func TestNextBackoffDelayJitterRange(t *testing.T) {
	testlog.Start(t)
	cfg := BackoffConfig{
		InitialDelay: 250 * time.Millisecond,
		Multiplier:   2.0,
		MaxDelay:     5 * time.Second,
		Jitter:       true,
	}
	rng := rand.New(rand.NewSource(7))
	got := NextBackoffDelay(cfg, 1, rng)
	if got < 125*time.Millisecond || got > 375*time.Millisecond {
		t.Fatalf("jitter out of range: %v", got)
	}
}

func TestValidateClientTransportProductionRequiresTLSMTLS(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultConfig()
	cfg.SecurityMode = SecurityModeProduction
	if err := cfg.ValidateClientTransport(); !errors.Is(err, ErrTLSRequired) {
		t.Fatalf("expected ErrTLSRequired, got %v", err)
	}

	cfg.TLS.Enabled = true
	if err := cfg.ValidateClientTransport(); !errors.Is(err, ErrMTLSRequired) {
		t.Fatalf("expected ErrMTLSRequired, got %v", err)
	}
}

func TestValidateClientTransportMutualRequiresCertKeyCA(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultConfig()
	cfg.TLS.Enabled = true
	cfg.TLS.Mutual = true
	if err := cfg.ValidateClientTransport(); !errors.Is(err, ErrTLSCAFileRequired) {
		t.Fatalf("expected ErrTLSCAFileRequired, got %v", err)
	}

	cfg.TLS.CAFile = "/tmp/ca.pem"
	if err := cfg.ValidateClientTransport(); !errors.Is(err, ErrTLSCertFileRequired) {
		t.Fatalf("expected ErrTLSCertFileRequired, got %v", err)
	}

	cfg.TLS.CertFile = "/tmp/client.pem"
	if err := cfg.ValidateClientTransport(); !errors.Is(err, ErrTLSKeyFileRequired) {
		t.Fatalf("expected ErrTLSKeyFileRequired, got %v", err)
	}

	cfg.TLS.KeyFile = "/tmp/client.key"
	if err := cfg.ValidateClientTransport(); err != nil {
		t.Fatalf("expected valid transport config, got %v", err)
	}
}

func TestValidateServerTransportProductionRequiresTLSMTLS(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultConfig()
	cfg.SecurityMode = SecurityModeProduction
	if err := cfg.ValidateServerTransport(); !errors.Is(err, ErrTLSRequired) {
		t.Fatalf("expected ErrTLSRequired, got %v", err)
	}

	cfg.TLS.Enabled = true
	if err := cfg.ValidateServerTransport(); !errors.Is(err, ErrMTLSRequired) {
		t.Fatalf("expected ErrMTLSRequired, got %v", err)
	}
}

// TestDefaultConfigMatchesReliabilityContract verifies timeout/backoff defaults from reliability.toml.
func TestDefaultConfigMatchesReliabilityContract(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultConfig()
	logs.Infof("test default config=%+v", cfg)

	if cfg.ConnectTimeout != 5*time.Second {
		t.Fatalf("connect timeout mismatch: got=%v want=%v", cfg.ConnectTimeout, 5*time.Second)
	}
	if cfg.HandshakeTimeout != 5*time.Second {
		t.Fatalf("handshake timeout mismatch: got=%v want=%v", cfg.HandshakeTimeout, 5*time.Second)
	}
	if cfg.ReadTimeout != 15*time.Second {
		t.Fatalf("read timeout mismatch: got=%v want=%v", cfg.ReadTimeout, 15*time.Second)
	}
	if cfg.WriteTimeout != 15*time.Second {
		t.Fatalf("write timeout mismatch: got=%v want=%v", cfg.WriteTimeout, 15*time.Second)
	}
	if cfg.HeartbeatInterval != 5*time.Second {
		t.Fatalf("heartbeat interval mismatch: got=%v want=%v", cfg.HeartbeatInterval, 5*time.Second)
	}
	if cfg.SessionDeadAfter != 15*time.Second {
		t.Fatalf("session dead-after mismatch: got=%v want=%v", cfg.SessionDeadAfter, 15*time.Second)
	}
	if cfg.AckTimeout != 20*time.Second {
		t.Fatalf("ack timeout mismatch: got=%v want=%v", cfg.AckTimeout, 20*time.Second)
	}

	if cfg.Backoff.InitialDelay != 250*time.Millisecond {
		t.Fatalf("initial backoff mismatch: got=%v want=%v", cfg.Backoff.InitialDelay, 250*time.Millisecond)
	}
	if cfg.Backoff.Multiplier != 2.0 {
		t.Fatalf("backoff multiplier mismatch: got=%v want=2.0", cfg.Backoff.Multiplier)
	}
	if cfg.Backoff.MaxDelay != 5*time.Second {
		t.Fatalf("max backoff mismatch: got=%v want=%v", cfg.Backoff.MaxDelay, 5*time.Second)
	}
	if !cfg.Backoff.Jitter {
		t.Fatalf("expected jitter enabled by default")
	}
}

// TestWithDefaultsPreservesExplicitOverrides verifies only unset values are defaulted.
func TestWithDefaultsPreservesExplicitOverrides(t *testing.T) {
	testlog.Start(t)

	in := Config{
		ConnectTimeout:   3 * time.Second,
		HandshakeTimeout: 4 * time.Second,
		SecurityMode:     "PRODUCTION",
		TLS: TLSConfig{
			Enabled: true,
			Mutual:  true,
		},
		Backoff: BackoffConfig{
			InitialDelay: 100 * time.Millisecond,
			Multiplier:   3.0,
			MaxDelay:     2 * time.Second,
			Jitter:       false,
		},
	}
	out := in.WithDefaults()
	logs.Infof("test with defaults input=%+v output=%+v", in, out)

	if out.ConnectTimeout != 3*time.Second {
		t.Fatalf("connect timeout override not preserved: %v", out.ConnectTimeout)
	}
	if out.HandshakeTimeout != 4*time.Second {
		t.Fatalf("handshake timeout override not preserved: %v", out.HandshakeTimeout)
	}
	if out.ReadTimeout != 15*time.Second || out.WriteTimeout != 15*time.Second {
		t.Fatalf("expected default read/write timeouts, got read=%v write=%v", out.ReadTimeout, out.WriteTimeout)
	}
	if out.SecurityMode != SecurityModeProduction {
		t.Fatalf("expected normalized production mode, got=%q", out.SecurityMode)
	}
	if out.Backoff.InitialDelay != 100*time.Millisecond || out.Backoff.Multiplier != 3.0 || out.Backoff.MaxDelay != 2*time.Second {
		t.Fatalf("backoff overrides not preserved: %+v", out.Backoff)
	}
	if out.Backoff.Jitter {
		t.Fatalf("expected explicit jitter=false preserved")
	}
}

// TestReadRegistrationRejectsOversizeControlEnvelope verifies handshake control message size bounds.
func TestReadRegistrationRejectsOversizeControlEnvelope(t *testing.T) {
	testlog.Start(t)

	var buf bytes.Buffer
	for i := 0; i < 129*1024; i++ {
		buf.WriteByte('x')
	}
	buf.WriteByte('\n')

	logs.Infof("test reading control envelope len=%d", buf.Len())
	_, err := ReadRegistration(bufio.NewReader(&buf))
	if !errors.Is(err, ErrControlMessageTooLarge) {
		t.Fatalf("expected ErrControlMessageTooLarge, got %v", err)
	}
}

// TestReadRegistrationRejectsMissingSeedDescription verifies handshake required seed fields.
func TestReadRegistrationRejectsMissingSeedDescription(t *testing.T) {
	testlog.Start(t)

	payload := []byte(`{"type":"seed.register","registration":{"ghost_id":"ghost.alpha","peer_identity":"ghost.alpha","seed_list":[{"id":"seed.flow","name":"Flow","description":""}]}}` + "\n")
	logs.Infof("test raw registration payload=%s", string(payload))
	_, err := ReadRegistration(bufio.NewReader(bytes.NewReader(payload)))
	if !errors.Is(err, ErrInvalidRegistration) {
		t.Fatalf("expected ErrInvalidRegistration, got %v", err)
	}
}

// TestReadRegistrationAckRejectsUnexpectedType verifies control-plane type discrimination.
func TestReadRegistrationAckRejectsUnexpectedType(t *testing.T) {
	testlog.Start(t)

	payload := []byte(`{"type":"seed.register","registration":{"ghost_id":"ghost.alpha","seed_list":[]}}` + "\n")
	logs.Infof("test raw ack payload with wrong type=%s", string(payload))
	_, err := ReadRegistrationAck(bufio.NewReader(bytes.NewReader(payload)))
	if !errors.Is(err, ErrInvalidRegistrationAck) {
		t.Fatalf("expected ErrInvalidRegistrationAck, got %v", err)
	}
}

// TestEventOutboxWhitespaceKeyNormalization verifies stable event_id key normalization behavior.
func TestEventOutboxWhitespaceKeyNormalization(t *testing.T) {
	testlog.Start(t)

	o := NewEventOutbox()
	now := time.Unix(1700000000, 0)
	o.Upsert(PendingEvent{
		EventID:       "  evt.norm.1  ",
		CommandID:     "cmd.1",
		GhostID:       "ghost.alpha",
		QueuedAt:      now,
		AckDeadlineAt: now.Add(20 * time.Second),
	})
	logs.Infof("test inserted pending event with padded key")

	if _, ok := o.Get("evt.norm.1"); !ok {
		t.Fatalf("expected trimmed key lookup to succeed")
	}
	if _, ok := o.Get("  evt.norm.1  "); !ok {
		t.Fatalf("expected padded key lookup to succeed due to normalization")
	}
}

// TestEventOutboxListDeterministicOrdering verifies sorted snapshots for retry workers and debug readability.
func TestEventOutboxListDeterministicOrdering(t *testing.T) {
	testlog.Start(t)

	o := NewEventOutbox()
	now := time.Unix(1700000010, 0)
	for _, id := range []string{"evt.3", "evt.1", "evt.2"} {
		o.Upsert(PendingEvent{
			EventID:       id,
			CommandID:     "cmd." + id,
			GhostID:       "ghost.alpha",
			QueuedAt:      now,
			AckDeadlineAt: now.Add(20 * time.Second),
		})
	}

	list := o.List()
	logs.Infof("test sorted outbox snapshot=%+v", list)
	if len(list) != 3 {
		t.Fatalf("unexpected list length: %d", len(list))
	}
	if list[0].EventID != "evt.1" || list[1].EventID != "evt.2" || list[2].EventID != "evt.3" {
		t.Fatalf("unexpected sort order: %+v", list)
	}
}
