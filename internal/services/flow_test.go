package services

import (
	"testing"

	logs "github.com/danmuck/smplog"
)

func TestFlowServiceDetailedControlFlow(t *testing.T) {
	flow := &FlowService{}
	logFlowf("flow/init: flow service created")

	status, err := flow.Status()
	if err != nil {
		t.Fatalf("status before flow: %v", err)
	}
	before := status.(FlowStatus)
	logFlowf("flow/status-before: next=%d intent=%d command=%d event=%d corr=%d",
		before.NextMessageID, before.LastIntentID, before.LastCommandID, before.LastEventID, before.LastCorrelationID)

	actions := flow.Actions()
	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(actions))
	}

	intentOut, err := actions["intent"]()
	if err != nil {
		t.Fatalf("intent action failed: %v", err)
	}
	logFlowf("ghost->seed rpc action=intent result=%q", intentOut)
	intentSnap := flow.Snapshot()
	logSnapshot("after-intent", intentSnap)

	commandOut, err := actions["command"]()
	if err != nil {
		t.Fatalf("command action failed: %v", err)
	}
	logFlowf("ghost->seed rpc action=command result=%q", commandOut)
	commandSnap := flow.Snapshot()
	logSnapshot("after-command", commandSnap)

	eventOut, err := actions["event"]()
	if err != nil {
		t.Fatalf("event action failed: %v", err)
	}
	logFlowf("ghost->seed rpc action=event result=%q", eventOut)
	eventSnap := flow.Snapshot()
	logSnapshot("after-event", eventSnap)

	if intentSnap.Intent == nil || commandSnap.Command == nil || eventSnap.Event == nil {
		t.Fatalf("expected intent, command, and event messages to be populated")
	}
	if commandSnap.Status.LastCorrelationID != intentSnap.Status.LastIntentID {
		t.Fatalf("expected command correlation id to point to intent id")
	}
	if eventSnap.Status.LastCorrelationID != intentSnap.Status.LastIntentID {
		t.Fatalf("expected event correlation id to remain intent id")
	}
	if eventSnap.Status.LastEventID != eventSnap.Event.MessageID {
		t.Fatalf("expected status last event id to match snapshot event id")
	}
	logFlowf("flow/assertions: correlation and status checks passed")
}

func TestFlowServiceErrorPathsThenRecovery(t *testing.T) {
	flow := &FlowService{}
	actions := flow.Actions()

	if _, err := actions["command"](); err == nil {
		t.Fatalf("expected command to fail before intent")
	} else {
		logFlowf("flow/error-path: command before intent failed as expected: %v", err)
	}
	if _, err := actions["event"](); err == nil {
		t.Fatalf("expected event to fail before command")
	} else {
		logFlowf("flow/error-path: event before command failed as expected: %v", err)
	}

	out, err := actions["flow-demo"]()
	if err != nil {
		t.Fatalf("flow-demo failed: %v", err)
	}
	if out != "flow complete" {
		t.Fatalf("expected flow complete output, got %q", out)
	}
	logFlowf("flow/recovery: flow-demo completed output=%q", out)

	snap := flow.Snapshot()
	logSnapshot("after-flow-demo", snap)
	if snap.Intent == nil || snap.Command == nil || snap.Event == nil {
		t.Fatalf("expected full flow snapshot after flow-demo")
	}
}

func logSnapshot(stage string, snap FlowSnapshot) {
	logFlowf("%s status next=%d intent=%d command=%d event=%d corr=%d",
		stage,
		snap.Status.NextMessageID,
		snap.Status.LastIntentID,
		snap.Status.LastCommandID,
		snap.Status.LastEventID,
		snap.Status.LastCorrelationID,
	)
	logMessage(stage, "intent", snap.Intent)
	logMessage(stage, "command", snap.Command)
	logMessage(stage, "event", snap.Event)
}

func logMessage(stage, label string, msg *MessageShape) {
	if msg == nil {
		logFlowf("%s %s-message=<nil>", stage, label)
		return
	}
	logFlowf("%s %s-message id=%d type=%d field-count=%d", stage, label, msg.MessageID, msg.MessageType, len(msg.Fields))
	for _, field := range msg.Fields {
		logFlowf("%s %s-field id=%d type=%d value=%s", stage, label, field.ID, field.Type, field.Value)
	}
}

func logFlowf(format string, v ...any) {
	logs.Logf(format, v...)
}
