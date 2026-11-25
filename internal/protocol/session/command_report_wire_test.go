package session

import (
	"bytes"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestCommandFrameRoundTrip(t *testing.T) {
	testlog.Start(t)

	in := Command{
		CommandID:    "cmd.1",
		IntentID:     "intent.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
		Args: map[string]string{
			"mode": "full",
		},
	}
	payload, err := EncodeCommandFrame(42, in)
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}

	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if fr.Header.MessageType != schema.MsgCommand {
		t.Fatalf("unexpected message type: %d", fr.Header.MessageType)
	}

	out, err := DecodeCommandFrame(fr)
	if err != nil {
		t.Fatalf("decode command: %v", err)
	}
	if out.CommandID != in.CommandID || out.IntentID != in.IntentID || out.GhostID != in.GhostID {
		t.Fatalf("command mismatch: in=%+v out=%+v", in, out)
	}
	if out.Args["mode"] != "full" {
		t.Fatalf("args mismatch: %+v", out.Args)
	}
}

func TestReportFrameRoundTrip(t *testing.T) {
	testlog.Start(t)

	in := Report{
		IntentID:        "intent.1",
		Phase:           "complete",
		Summary:         "intent satisfied",
		CompletionState: "satisfied",
		CommandID:       "cmd.1",
		ExecutionID:     "exec.1",
		EventID:         "evt.1",
		Outcome:         "success",
		TimestampMS:     1760000000000,
	}
	payload, err := EncodeReportFrame(77, in)
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}

	fr, err := frame.ReadFrame(bytes.NewReader(payload), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if fr.Header.MessageType != schema.MsgReport {
		t.Fatalf("unexpected message type: %d", fr.Header.MessageType)
	}

	out, err := DecodeReportFrame(fr)
	if err != nil {
		t.Fatalf("decode report: %v", err)
	}
	if out.IntentID != in.IntentID || out.Phase != in.Phase || out.Summary != in.Summary {
		t.Fatalf("report mismatch: in=%+v out=%+v", in, out)
	}
	if out.EventID != in.EventID || out.TimestampMS != in.TimestampMS {
		t.Fatalf("report correlation mismatch: in=%+v out=%+v", in, out)
	}
}

// A required field that is present but blank passes the TLV schema check; the decoder must apply
// the encoder's required-field rule so anything that decodes also re-encodes (errors.toml 1300).
func TestDecodeRejectsBlankRequiredFields(t *testing.T) {
	testlog.Start(t)
	str := func(id uint16, v string) tlv.Field {
		return tlv.Field{ID: id, Type: tlv.TypeString, Value: []byte(v)}
	}
	read := func(raw []byte, err error) frame.Frame {
		t.Helper()
		if err != nil {
			t.Fatalf("encode: %v", err)
		}
		fr, err := frame.ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
		if err != nil {
			t.Fatalf("read frame: %v", err)
		}
		return fr
	}
	check := func(name string, err error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: expected rejection", name)
		}
		if code := ErrorCode(err); code != schema.ErrCodeSemanticValidation {
			t.Fatalf("%s: expected code %d, got %d (%v)", name, schema.ErrCodeSemanticValidation, code, err)
		}
	}

	cmd := read(encodeFrameForMessage(1, schema.MsgCommand, 0, []tlv.Field{
		str(schema.FieldCommandID, " "), str(schema.FieldIntentID, "intent.1"), str(schema.FieldGhostID, "ghost.alpha"),
		str(schema.FieldSeedSelector, "seed.flow"), str(schema.FieldOperation, "status"),
	}))
	_, err := DecodeCommandFrame(cmd)
	check("command", err)

	ev := read(encodeFrameForMessage(2, schema.MsgEvent, 0, []tlv.Field{
		str(schema.FieldEventID, "evt.1"), str(schema.FieldCommandID, ""), str(schema.FieldIntentID, "intent.1"),
		str(schema.FieldGhostID, "ghost.alpha"), str(schema.FieldSeedID, "seed.flow"), str(schema.FieldOutcome, "success"),
	}))
	_, err = DecodeEventFrame(ev)
	check("event", err)

	rep := read(encodeFrameForMessage(3, schema.MsgReport, 0, []tlv.Field{
		str(schema.FieldIntentID, "intent.1"), str(schema.FieldPhase, ""), str(schema.FieldSummary, "s"),
		str(schema.FieldCompletionState, "satisfied"),
	}))
	_, err = DecodeReportFrame(rep)
	check("report", err)
}
