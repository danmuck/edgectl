package main

import (
	"strings"
	"testing"
)

// TestDefaultInteractiveRunConfig verifies interactive mode keeps paused pacing.
func TestDefaultInteractiveRunConfig(t *testing.T) {
	cfg := defaultInteractiveRunConfig()
	if cfg.pacing != pacingPause {
		t.Fatalf("expected pacing %q, got %q", pacingPause, cfg.pacing)
	}
	if cfg.pauseDelay <= 0 {
		t.Fatalf("expected positive pauseDelay, got %v", cfg.pauseDelay)
	}
}

// TestDefaultRunModeConfig verifies non-interactive run mode uses free pacing.
func TestDefaultRunModeConfig(t *testing.T) {
	cfg := defaultRunModeConfig()
	if cfg.pacing != pacingFree {
		t.Fatalf("expected pacing %q, got %q", pacingFree, cfg.pacing)
	}
	if cfg.pauseDelay != 0 {
		t.Fatalf("expected zero pauseDelay in free mode, got %v", cfg.pauseDelay)
	}
}

// TestFormatMessageTypeLineSchemaValidate verifies message_type ids are rendered with envelope labels.
func TestFormatMessageTypeLineSchemaValidate(t *testing.T) {
	in := "INFO schema.Validate ok message_type=5"
	out, ok := formatMessageTypeLine(in)
	if !ok {
		t.Fatalf("expected formatted=true")
	}
	if !strings.Contains(out, "message_type=event(id=5)") {
		t.Fatalf("unexpected formatted output: %q", out)
	}
}

// TestFormatMessageTypeLineFrameWrite verifies frame logs get descriptive message type labels.
func TestFormatMessageTypeLineFrameWrite(t *testing.T) {
	in := "INFO frame.WriteFrame ok message_id=42 message_type=8 payload_len=100 auth_len=0"
	out, ok := formatMessageTypeLine(in)
	if !ok {
		t.Fatalf("expected formatted=true")
	}
	if !strings.Contains(out, "message_type=event.ack(id=8)") {
		t.Fatalf("unexpected formatted output: %q", out)
	}
}

// TestFormatMessageTypeLineUnknownType verifies unknown message types are still labeled deterministically.
func TestFormatMessageTypeLineUnknownType(t *testing.T) {
	in := "INFO frame.ReadFrame ok message_id=42 message_type=99 payload_len=10 auth_len=0"
	out, ok := formatMessageTypeLine(in)
	if !ok {
		t.Fatalf("expected formatted=true")
	}
	if !strings.Contains(out, "message_type=message_type_99(id=99)") {
		t.Fatalf("unexpected formatted output: %q", out)
	}
}
