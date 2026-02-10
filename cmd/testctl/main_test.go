package main

import (
	"strings"
	"testing"
)

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
