package ghost

import (
	"encoding/binary"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func newCompatAdminService(t *testing.T) *Service {
	t.Helper()
	svc := NewServiceWithConfig(DefaultServiceConfig())
	svc.server = newRadiatingServer(t, "ghost.alpha")
	return svc
}

func compatCommandFrame(t *testing.T, commandID string, unknown []tlv.Field) []byte {
	t.Helper()
	raw, err := session.EncodeCommandFrame(901, session.Command{
		CommandID:     commandID,
		IntentID:      "intent.compat",
		GhostID:       "ghost.alpha",
		SeedSelector:  "seed.flow",
		Operation:     "status",
		UnknownFields: unknown,
	})
	if err != nil {
		t.Fatalf("encode command frame: %v", err)
	}
	return raw
}

func assertAdminFailure(t *testing.T, resp controlResponse, wantCode uint32) {
	t.Helper()
	if resp.OK {
		t.Fatalf("expected failure, got OK")
	}
	if resp.ErrorCode != wantCode {
		t.Fatalf("error_code = %d (%s), want %d", resp.ErrorCode, resp.Error, wantCode)
	}
}

// Admin boundary version gate: an unsupported protocol version is rejected with 1100 and not executed.
func TestAdminExecuteEnvelopeRejectsUnsupportedVersion(t *testing.T) {
	testlog.Start(t)
	svc := newCompatAdminService(t)
	raw := compatCommandFrame(t, "cmd.compat.version", nil)
	binary.BigEndian.PutUint16(raw[4:6], 2)

	resp := svc.handleControlRequest(controlRequest{Action: "execute_envelope", CommandFrame: raw}, "")
	assertAdminFailure(t, resp, schema.ErrCodeFramingInvalidHeader)
	if _, ok := svc.server.ExecutionByCommandID("cmd.compat.version"); ok {
		t.Fatalf("rejected frame must not execute")
	}
}

func TestAdminExecuteEnvelopeRejectsUnknownFlagBits(t *testing.T) {
	testlog.Start(t)
	svc := newCompatAdminService(t)
	raw := compatCommandFrame(t, "cmd.compat.flags", nil)
	flags := binary.BigEndian.Uint32(raw[20:24]) | 0x08
	binary.BigEndian.PutUint32(raw[20:24], flags)

	resp := svc.handleControlRequest(controlRequest{Action: "execute_envelope", CommandFrame: raw}, "")
	assertAdminFailure(t, resp, schema.ErrCodeFramingInvalidHeader)
	if _, ok := svc.server.ExecutionByCommandID("cmd.compat.flags"); ok {
		t.Fatalf("rejected frame must not execute")
	}
}

// Unknown TLV fields reach Ghost but never change what executes.
func TestAdminExecuteEnvelopeUnknownFieldsAreInert(t *testing.T) {
	testlog.Start(t)
	svc := newCompatAdminService(t)
	unknown := []tlv.Field{
		{ID: 9000, Type: tlv.TypeString, Value: []byte("seed.mongod")},
		{ID: 9001, Type: tlv.TypeString, Value: []byte("stop")},
	}
	resp := svc.handleControlRequest(controlRequest{
		Action:       "execute_envelope",
		CommandFrame: compatCommandFrame(t, "cmd.compat.unknown", unknown),
	}, "")
	if !resp.OK {
		t.Fatalf("command with unknown fields failed: %s (code %d)", resp.Error, resp.ErrorCode)
	}
	state, ok := svc.server.ExecutionByCommandID("cmd.compat.unknown")
	if !ok {
		t.Fatalf("execution missing")
	}
	if state.SeedSelector != "seed.flow" || state.Operation != "status" || state.Outcome != OutcomeSuccess {
		t.Fatalf("unknown fields altered execution: seed=%q op=%q outcome=%q", state.SeedSelector, state.Operation, state.Outcome)
	}
}

func TestAdminFailureCodesForRequestErrors(t *testing.T) {
	testlog.Start(t)
	svc := newCompatAdminService(t)

	assertAdminFailure(t, svc.handleControlRequest(controlRequest{Action: "no_such_action"}, ""), schema.ErrCodeSemanticValidation)
	assertAdminFailure(t, svc.handleControlRequest(controlRequest{Action: "execute_envelope"}, ""), schema.ErrCodeSemanticValidation)

	// Required field missing: decodes but fails schema validation.
	missing := tlv.EncodeFields([]tlv.Field{
		{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.compat.missing")},
	})
	raw, err := session.EncodeCommandFrame(902, session.Command{
		CommandID: "x", IntentID: "x", GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status",
	})
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	header := raw[:32]
	binary.BigEndian.PutUint64(header[24:32], uint64(len(missing)))
	resp := svc.handleControlRequest(controlRequest{Action: "execute_envelope", CommandFrame: append(append([]byte{}, header...), missing...)}, "")
	assertAdminFailure(t, resp, schema.ErrCodeSemanticValidation)
}

func TestAdminFailureCodeForFrameAuth(t *testing.T) {
	testlog.Start(t)
	cfg := DefaultServiceConfig()
	cfg.AdminFrameAuthToken = "secret"
	svc := NewServiceWithConfig(cfg)
	svc.server = newRadiatingServer(t, "ghost.alpha")

	resp := svc.handleControlRequest(controlRequest{
		Action:       "execute_envelope",
		CommandFrame: compatCommandFrame(t, "cmd.compat.auth", nil),
	}, "")
	assertAdminFailure(t, resp, schema.ErrCodeTransportFailure)
}
