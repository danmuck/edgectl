package mirage

import (
	"bufio"
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Registers ghost.alpha over raw TCP and returns the live connection and reader.
func rawRegisteredSession(t *testing.T, addr string) (net.Conn, *bufio.Reader) {
	t.Helper()
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if err := session.WriteRegistration(conn, session.Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
		SeedList:     []session.SeedInfo{{ID: "seed.flow", Name: "Flow", Description: "flow"}},
	}); err != nil {
		t.Fatalf("write registration: %v", err)
	}
	reader := bufio.NewReader(conn)
	ack, err := session.ReadRegistrationAck(reader)
	if err != nil || ack.Status != session.AckStatusAccepted {
		t.Fatalf("registration: ack=%+v err=%v", ack, err)
	}
	return conn, reader
}

// Session boundary version gate: a frame with an unsupported version closes the session.
func TestServiceSessionClosesOnUnsupportedVersionFrame(t *testing.T) {
	testlog.Start(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg := DefaultServiceConfig()
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second
	cfg.Session.HandshakeTimeout = 2 * time.Second
	svc := NewServiceWithConfig(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Serve(ctx, ln) }()

	for _, tc := range []struct {
		name   string
		mutate func(raw []byte)
	}{
		{"version", func(raw []byte) { binary.BigEndian.PutUint16(raw[4:6], frame.ProtocolVersion+1) }},
		{"flags", func(raw []byte) { binary.BigEndian.PutUint32(raw[20:24], 0x08) }},
	} {
		conn, reader := rawRegisteredSession(t, ln.Addr().String())
		raw, err := session.EncodeEventFrame(1, session.Event{
			EventID: "evt.compat." + tc.name, CommandID: "cmd.compat", IntentID: "intent.compat",
			GhostID: "ghost.alpha", SeedID: "seed.flow", Outcome: OutcomeSuccess,
		})
		if err != nil {
			t.Fatalf("%s: encode event: %v", tc.name, err)
		}
		tc.mutate(raw)
		if _, err := conn.Write(raw); err != nil {
			t.Fatalf("%s: write frame: %v", tc.name, err)
		}
		// No event.ack is sent; Mirage closes the stream.
		if _, err := reader.ReadByte(); err == nil {
			t.Fatalf("%s: expected session close, got data", tc.name)
		}
		_ = conn.Close()
		for _, g := range svc.server.SnapshotRegisteredGhosts() {
			if g.EventCount != 0 {
				t.Fatalf("%s: rejected frame was counted as an event", tc.name)
			}
		}
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}

func TestMirageAdminFailureCodes(t *testing.T) {
	testlog.Start(t)
	svc := NewServiceWithConfig(DefaultServiceConfig())

	cases := []struct {
		name string
		req  adminControlRequest
		want uint32
	}{
		{"unknown action", adminControlRequest{Action: "no_such_action"}, schema.ErrCodeSemanticValidation},
		{"invalid issue", adminControlRequest{Action: "submit_issue"}, schema.ErrCodeSemanticValidation},
		{"reconcile missing intent_id", adminControlRequest{Action: "reconcile_intent"}, schema.ErrCodeSemanticValidation},
		{"reconcile unknown intent", adminControlRequest{Action: "reconcile_intent", IntentID: "intent.none"}, schema.ErrCodeSemanticValidation},
		{"attach missing addr", adminControlRequest{Action: "attach_ghost_admin"}, schema.ErrCodeSemanticValidation},
	}
	for _, tc := range cases {
		resp := svc.handleAdminControlRequest(tc.req)
		if resp.OK || resp.ErrorCode != tc.want {
			t.Errorf("%s: ok=%v error_code=%d (%s), want %d", tc.name, resp.OK, resp.ErrorCode, resp.Error, tc.want)
		}
	}
}

// A code reported by a Ghost admin endpoint passes through Mirage unchanged.
func TestMirageAdminPassesThroughGhostErrorCode(t *testing.T) {
	testlog.Start(t)
	err := fmt.Errorf("dispatch: %w", &GhostAdminError{Action: "execute_envelope", Code: schema.ErrCodeFramingInvalidHeader, Message: "unsupported version"})
	if got := adminErrorCode(err); got != schema.ErrCodeFramingInvalidHeader {
		t.Fatalf("adminErrorCode = %d, want %d", got, schema.ErrCodeFramingInvalidHeader)
	}
}
