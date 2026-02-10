package ghost

import (
	"bytes"
	"encoding/json"
	"strings"
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

func TestHandleControlRequestExecuteLegacyActionRejected(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	resp := svc.handleControlRequest(controlRequest{Action: "execute"})
	if resp.OK {
		t.Fatalf("expected execute action rejection")
	}
	if !strings.Contains(resp.Error, "unknown action: execute") {
		t.Fatalf("unexpected execute rejection error: %q", resp.Error)
	}
}

func TestHandleControlRequestExecuteEnvelopeAuthRequired(t *testing.T) {
	testlog.Start(t)

	cfg := DefaultServiceConfig()
	cfg.AdminFrameAuthToken = "phase6-token"
	svc := NewServiceWithConfig(cfg)
	svc.server = newRadiatingServer(t, "ghost.alpha")

	commandFrame, err := session.EncodeCommandFrame(702, session.Command{
		CommandID:    "cmd.intent.702.1",
		IntentID:     "intent.702",
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
	if resp.OK {
		t.Fatalf("expected execute_envelope auth failure")
	}
	if !strings.Contains(resp.Error, ErrAdminFrameAuthRequired.Error()) {
		t.Fatalf("expected auth required error, got: %q", resp.Error)
	}
}

func TestHandleControlRequestExecuteEnvelopeAuthAccepted(t *testing.T) {
	testlog.Start(t)

	cfg := DefaultServiceConfig()
	cfg.AdminFrameAuthToken = "phase6-token"
	svc := NewServiceWithConfig(cfg)
	svc.server = newRadiatingServer(t, "ghost.alpha")

	commandFrame, err := session.EncodeCommandFrameWithAuth(703, session.Command{
		CommandID:    "cmd.intent.703.1",
		IntentID:     "intent.703",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
	}, []byte("phase6-token"))
	if err != nil {
		t.Fatalf("encode command frame: %v", err)
	}

	resp := svc.handleControlRequest(controlRequest{
		Action:       "execute_envelope",
		CommandFrame: commandFrame,
	})
	if !resp.OK {
		t.Fatalf("execute_envelope with auth failed: %s", resp.Error)
	}
}

func TestHandleControlRequestListSeedCatalog(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	svc.server = newRadiatingServer(t, "ghost.alpha")

	resp := svc.handleControlRequest(controlRequest{Action: "list_seed_catalog"})
	if !resp.OK {
		t.Fatalf("list_seed_catalog failed: %s", resp.Error)
	}

	raw, err := json.Marshal(resp.Data)
	if err != nil {
		t.Fatalf("marshal seed catalog response: %v", err)
	}
	var catalog []SeedCapability
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("decode seed catalog response: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("unexpected seed catalog size: %d", len(catalog))
	}
	entry := catalog[0]
	if entry.Metadata.ID != "seed.flow" {
		t.Fatalf("unexpected seed id: %q", entry.Metadata.ID)
	}
	if len(entry.Operations) == 0 {
		t.Fatalf("expected operations in seed capability")
	}
	if len(entry.CommandCatalog) == 0 {
		t.Fatalf("expected command catalog entries in seed capability")
	}
	for i := range entry.CommandCatalog {
		if entry.CommandCatalog[i].SeedSelector != entry.Metadata.ID {
			t.Fatalf("template seed selector mismatch: %+v", entry.CommandCatalog[i])
		}
	}
}
