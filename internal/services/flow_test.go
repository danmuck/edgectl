package services

import (
	"testing"
)

// TestFlowServiceLogsMessagePassing exercises the flow and logs each step.
func TestFlowServiceLogsMessagePassing(t *testing.T) {
	service := &FlowService{}

	actions := service.Actions()
	intent, ok := actions["intent"]
	if !ok {
		t.Fatalf("intent action missing")
	}
	command, ok := actions["command"]
	if !ok {
		t.Fatalf("command action missing")
	}
	event, ok := actions["event"]
	if !ok {
		t.Fatalf("event action missing")
	}

	if msg, err := intent(); err != nil {
		t.Fatalf("intent: %v", err)
	} else {
		logTestf(t, "intent -> %s", msg)
	}
	logStatus(t, service, "after intent")

	if msg, err := command(); err != nil {
		t.Fatalf("command: %v", err)
	} else {
		logTestf(t, "command -> %s", msg)
	}
	logStatus(t, service, "after command")

	if msg, err := event(); err != nil {
		t.Fatalf("event: %v", err)
	} else {
		logTestf(t, "event -> %s", msg)
	}
	logStatus(t, service, "after event")
}

// logStatus logs the current flow status for tracing.
func logStatus(t *testing.T, service *FlowService, stage string) {
	t.Helper()
	statusAny, err := service.Status()
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	status, ok := statusAny.(FlowStatus)
	if !ok {
		t.Fatalf("unexpected status type: %T", statusAny)
	}
	logTestf(t, "%s: next=%d intent=%d command=%d event=%d corr=%d",
		stage,
		status.NextMessageID,
		status.LastIntentID,
		status.LastCommandID,
		status.LastEventID,
		status.LastCorrelationID,
	)
}

func logTestf(t *testing.T, format string, v ...any) {
	t.Helper()
	t.Logf(format, v...)
}
