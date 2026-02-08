package ghost

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestHandleControlRequestExecuteEnvelope(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	svc.server = newRadiatingServer(t, "ghost.alpha")

	commandFrame, err := session.EncodeCommandFrame(701, session.Command{
		CommandID:    "cmd.intent.701.1",
		IntentID:     "intent.701",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("encode command frame: %v", err)
	}

	resp := svc.handleControlRequest(controlRequest{
		Action:       "execute_envelope",
		CommandFrame: commandFrame,
	})
	if !resp.OK {
		t.Fatalf("execute_envelope failed: %s", resp.Error)
	}

	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal response data: %v", err)
	}
	var out executeEnvelopeResponse
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode response data: %v", err)
	}
	eventFrame, err := frame.ReadFrame(bytes.NewReader(out.EventFrame), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read event frame: %v", err)
	}
	event, err := session.DecodeEventFrame(eventFrame)
	if err != nil {
		t.Fatalf("decode event frame: %v", err)
	}
	if event.CommandID != "cmd.intent.701.1" {
		t.Fatalf("unexpected command id: %q", event.CommandID)
	}
	if event.IntentID != "intent.701" {
		t.Fatalf("unexpected intent id: %q", event.IntentID)
	}
	if event.GhostID != "ghost.alpha" {
		t.Fatalf("unexpected ghost id: %q", event.GhostID)
	}
	if event.SeedID != "seed.flow" {
		t.Fatalf("unexpected seed id: %q", event.SeedID)
	}
	if event.Outcome != OutcomeSuccess {
		t.Fatalf("unexpected outcome: %q", event.Outcome)
	}
}

func TestHandleControlRequestExecuteEnvelopeMissingFrame(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	resp := svc.handleControlRequest(controlRequest{Action: "execute_envelope"})
	if resp.OK {
		t.Fatalf("expected missing command frame failure")
	}
}
