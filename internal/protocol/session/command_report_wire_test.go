package session

import (
	"bytes"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
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
