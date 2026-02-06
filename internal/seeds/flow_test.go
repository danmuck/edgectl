package seeds

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol"
	logs "github.com/danmuck/smplog"
)

func TestFlowSeedDetailedControlFlow(t *testing.T) {
	flow := &FlowSeed{}
	logShape("flow_contract", map[string]any{
		"topology": map[string]any{
			"ghost_servers": 1,
			"seed_servers":  1,
			"flow_path":     "mirage -> ghost.flow_seed",
		},
		"protocol": map[string]any{
			"magic_label":  "EDGE",
			"magic_value":  fmt.Sprintf("0x%08X", protocol.Magic),
			"version":      protocol.Version,
			"header_bytes": protocol.HeaderSize,
		},
		"seed_interface": map[string]any{
			"name": "seeds.Seed",
			"methods": map[string]any{
				"name":    "() -> string",
				"status":  "() -> any,error",
				"actions": "() -> map[action_name]action_fn",
			},
		},
		"action_fn": "() -> output,error",
		"message_type_enum": map[string]any{
			"intent":  int(protocol.MessageIntent),
			"command": int(protocol.MessageCommand),
			"event":   int(protocol.MessageEvent),
		},
		"field_type_enum": map[string]any{
			"string": int(protocol.FieldString),
			"u64":    int(protocol.FieldUint64),
		},
		"snapshot": map[string]any{
			"status":  "next_message_id,last_intent_id,last_command_id,last_event_id,last_correlation_id",
			"intent":  "message_shape or nil",
			"command": "message_shape or nil",
			"event":   "message_shape or nil",
		},
	})

	statusAny, err := flow.Status()
	if err != nil {
		t.Fatalf("status before flow: %v", err)
	}
	status := statusAny.(FlowStatus)
	logShape("flow_status_before", map[string]any{
		"next_message_id":     status.NextMessageID,
		"last_intent_id":      status.LastIntentID,
		"last_command_id":     status.LastCommandID,
		"last_event_id":       status.LastEventID,
		"last_correlation_id": status.LastCorrelationID,
	})

	actions := flow.Actions()
	if len(actions) != 4 {
		t.Fatalf("expected 4 actions, got %d", len(actions))
	}
	logShape("flow_actions", map[string]any{
		"item_1": "intent",
		"item_2": "command",
		"item_3": "event",
		"item_4": "flow-demo",
	})

	intentOut, err := actions["intent"]()
	if err != nil {
		t.Fatalf("intent action failed: %v", err)
	}
	logShape("flow_step", map[string]any{
		"from":   "mirage",
		"to":     "ghost.flow",
		"action": "intent",
		"result": intentOut,
	})
	intentSnap := flow.Snapshot()
	logShape("flow_snapshot_after_intent", snapshotShape(intentSnap))

	commandOut, err := actions["command"]()
	if err != nil {
		t.Fatalf("command action failed: %v", err)
	}
	logShape("flow_step", map[string]any{
		"from":   "mirage",
		"to":     "ghost.flow",
		"action": "command",
		"result": commandOut,
	})
	commandSnap := flow.Snapshot()
	logShape("flow_snapshot_after_command", snapshotShape(commandSnap))

	eventOut, err := actions["event"]()
	if err != nil {
		t.Fatalf("event action failed: %v", err)
	}
	logShape("flow_step", map[string]any{
		"from":   "mirage",
		"to":     "ghost.flow",
		"action": "event",
		"result": eventOut,
	})
	eventSnap := flow.Snapshot()
	logShape("flow_snapshot_after_event", snapshotShape(eventSnap))

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
}

func TestFlowSeedErrorPathsThenRecovery(t *testing.T) {
	flow := &FlowSeed{}
	actions := flow.Actions()

	if _, err := actions["command"](); err == nil {
		t.Fatalf("expected command to fail before intent")
	} else {
		logShape("flow_error", map[string]any{
			"action": "command",
			"when":   "before_intent",
			"error":  err.Error(),
		})
	}
	if _, err := actions["event"](); err == nil {
		t.Fatalf("expected event to fail before command")
	} else {
		logShape("flow_error", map[string]any{
			"action": "event",
			"when":   "before_command",
			"error":  err.Error(),
		})
	}

	out, err := actions["flow-demo"]()
	if err != nil {
		t.Fatalf("flow-demo failed: %v", err)
	}
	if out != "flow complete" {
		t.Fatalf("expected flow complete output, got %q", out)
	}
	logShape("flow_recovery", map[string]any{
		"action": "flow-demo",
		"result": out,
	})

	snap := flow.Snapshot()
	logShape("flow_snapshot_after_flow_demo", snapshotShape(snap))
	if snap.Intent == nil || snap.Command == nil || snap.Event == nil {
		t.Fatalf("expected full flow snapshot after flow-demo")
	}
}

func snapshotShape(snap FlowSnapshot) map[string]any {
	return map[string]any{
		"status": map[string]any{
			"next_message_id":     snap.Status.NextMessageID,
			"last_intent_id":      snap.Status.LastIntentID,
			"last_command_id":     snap.Status.LastCommandID,
			"last_event_id":       snap.Status.LastEventID,
			"last_correlation_id": snap.Status.LastCorrelationID,
		},
		"intent":  messageShapeMap(snap.Intent),
		"command": messageShapeMap(snap.Command),
		"event":   messageShapeMap(snap.Event),
	}
}

func messageShapeMap(msg *MessageShape) map[string]any {
	if msg == nil {
		return map[string]any{"value": "nil"}
	}
	fields := map[string]any{}
	for i, f := range msg.Fields {
		fields[fmt.Sprintf("field_%d", i+1)] = map[string]any{
			"id":            f.ID,
			"field_label":   flowFieldLabel(f.ID),
			"represents":    flowFieldMeaning(f.ID),
			"kind":          int(f.Type),
			"kind_label":    fieldTypeLabel(f.Type),
			"decoded_value": f.Value,
		}
	}
	return map[string]any{
		"message_id":         msg.MessageID,
		"message_type":       int(msg.MessageType),
		"message_type_label": messageTypeLabel(msg.MessageType),
		"fields":             fields,
	}
}

func messageTypeLabel(mt protocol.MessageType) string {
	switch mt {
	case protocol.MessageIntent:
		return "intent"
	case protocol.MessageCommand:
		return "command"
	case protocol.MessageEvent:
		return "event"
	default:
		return "unknown"
	}
}

func fieldTypeLabel(ft protocol.FieldType) string {
	switch ft {
	case protocol.FieldUint8:
		return "uint8"
	case protocol.FieldUint16:
		return "uint16"
	case protocol.FieldUint32:
		return "uint32"
	case protocol.FieldUint64:
		return "uint64"
	case protocol.FieldBool:
		return "bool"
	case protocol.FieldString:
		return "string"
	case protocol.FieldBytes:
		return "bytes"
	default:
		return "unknown"
	}
}

func flowFieldLabel(id uint16) string {
	switch id {
	case fieldIntentName:
		return "intent_name"
	case fieldIntentTarget:
		return "intent_target"
	case fieldCommandName:
		return "command_name"
	case fieldCommandTarget:
		return "command_target"
	case fieldCorrelationID:
		return "correlation_id"
	case fieldEventName:
		return "event_name"
	case fieldEventStatus:
		return "event_status"
	default:
		return "unknown_field"
	}
}

func flowFieldMeaning(id uint16) string {
	switch id {
	case fieldIntentName:
		return "requested high-level operation"
	case fieldIntentTarget:
		return "target node or domain for intent"
	case fieldCommandName:
		return "executable command derived from intent"
	case fieldCommandTarget:
		return "execution target for command"
	case fieldCorrelationID:
		return "links command/event back to originating intent"
	case fieldEventName:
		return "event emitted after command execution"
	case fieldEventStatus:
		return "event outcome status"
	default:
		return "unmapped semantic meaning"
	}
}

func logShape(label string, payload map[string]any) {
	lines := []string{label + ":"}
	lines = append(lines, formatKV(payload, 1)...)
	logs.Log(strings.Join(lines, "\n"))
}

func formatKV(m map[string]any, depth int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pad := strings.Repeat("    ", depth)
	out := make([]string, 0)
	for _, k := range keys {
		v := m[k]
		switch x := v.(type) {
		case map[string]any:
			out = append(out, fmt.Sprintf("%s%s:", pad, k))
			out = append(out, formatKV(x, depth+1)...)
		case string:
			out = append(out, fmt.Sprintf("%s%s: %q", pad, k, x))
		default:
			out = append(out, fmt.Sprintf("%s%s: %v", pad, k, x))
		}
	}
	return out
}
