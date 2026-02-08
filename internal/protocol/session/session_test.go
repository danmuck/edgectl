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
