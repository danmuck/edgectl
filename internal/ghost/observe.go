package ghost

import (
	"strings"

	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// Component name on every Ghost custody line.
const observeComponent = "ghost"

// Command received at the Ghost boundary: accepted, replayed (idempotent), or rejected.
func emitCommandIn(cmd CommandEnv, status string, errText string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(cmd.Peer),
		Direction:   observability.DirectionInbound,
		MessageID:   cmd.MessageID,
		MessageType: schema.MsgCommand,
		IntentID:    strings.TrimSpace(cmd.IntentID),
		CommandID:   strings.TrimSpace(cmd.CommandID),
		Status:      status,
		Error:       errText,
	})
}

// seed.execute handed to a seed; peer is the seed id.
func emitSeedExecute(state ExecutionState, exec SeedExecuteEnv) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(exec.SeedID),
		Direction:   observability.DirectionOutbound,
		MessageID:   state.MessageID,
		MessageType: schema.MsgSeedExecute,
		IntentID:    state.IntentID,
		CommandID:   state.CommandID,
		ExecutionID: exec.ExecutionID,
		Status:      observability.StatusDispatched,
	})
}

// seed.result returned by a seed; status is the seed status.
func emitSeedResult(state ExecutionState, result SeedResultEnv) {
	errText := ""
	if result.Status != SeedStatusOK {
		errText = strings.TrimSpace(string(result.Stderr))
	}
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(result.SeedID),
		Direction:   observability.DirectionInbound,
		MessageID:   state.MessageID,
		MessageType: schema.MsgSeedResult,
		IntentID:    state.IntentID,
		CommandID:   state.CommandID,
		ExecutionID: result.ExecutionID,
		Status:      result.Status,
		Error:       errText,
	})
}

// Terminal event leaving Ghost toward the command's sender; status is the event outcome.
func emitEventOut(messageID uint64, peer string, event EventEnv, executionID string, status string, errText string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(peer),
		Direction:   observability.DirectionOutbound,
		MessageID:   messageID,
		MessageType: schema.MsgEvent,
		IntentID:    event.IntentID,
		CommandID:   event.CommandID,
		ExecutionID: executionID,
		EventID:     event.EventID,
		Status:      status,
		Error:       errText,
	})
}

// event.ack received from Mirage on the session.
func emitEventAckIn(messageID uint64, peer string, ack session.EventAck, intentID string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(peer),
		Direction:   observability.DirectionInbound,
		MessageID:   messageID,
		MessageType: schema.MsgEventAck,
		IntentID:    intentID,
		CommandID:   ack.CommandID,
		EventID:     ack.EventID,
		AckStatus:   ack.AckStatus,
		Status:      ack.AckStatus,
	})
}

// Peer placeholder so a missing identity is explicit rather than an empty required field.
func peerOrUnknown(peer string) string {
	if p := strings.TrimSpace(peer); p != "" {
		return p
	}
	return "unknown"
}
