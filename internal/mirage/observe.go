package mirage

import (
	"strings"

	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// Component name on every Mirage custody line.
const observeComponent = "mirage"

// Issue accepted from the user boundary (issue -> Mirage).
func emitIssue(messageID uint64, issue IssueEnv) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(issue.Actor),
		Direction:   observability.DirectionInbound,
		MessageID:   messageID,
		MessageType: schema.MsgIssue,
		IntentID:    issue.IntentID,
		Status:      observability.StatusAccepted,
	})
}

// Command dispatch, retry, or command terminal state toward a Ghost.
func emitCommand(messageID uint64, cmd session.Command, status string, errText string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(cmd.GhostID),
		Direction:   observability.DirectionOutbound,
		MessageID:   messageID,
		MessageType: schema.MsgCommand,
		IntentID:    cmd.IntentID,
		CommandID:   cmd.CommandID,
		Status:      status,
		Error:       errText,
	})
}

// Terminal event received from a Ghost (reconcile return path or session).
func emitEventIn(messageID uint64, peer string, event session.Event, status string, errText string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(peer),
		Direction:   observability.DirectionInbound,
		MessageID:   messageID,
		MessageType: schema.MsgEvent,
		IntentID:    event.IntentID,
		CommandID:   event.CommandID,
		EventID:     event.EventID,
		Status:      status,
		Error:       errText,
	})
}

// event.ack sent back to a Ghost on the session.
func emitEventAckOut(messageID uint64, ack session.EventAck, intentID string) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(ack.GhostID),
		Direction:   observability.DirectionOutbound,
		MessageID:   messageID,
		MessageType: schema.MsgEventAck,
		IntentID:    intentID,
		CommandID:   ack.CommandID,
		EventID:     ack.EventID,
		AckStatus:   ack.AckStatus,
		Status:      ack.AckStatus,
	})
}

// Report emitted to the user boundary; status is the report completion_state.
func emitReport(messageID uint64, actor string, report session.Report) {
	observability.Emit(observability.LogEvent{
		Component:   observeComponent,
		Peer:        peerOrUnknown(actor),
		Direction:   observability.DirectionOutbound,
		MessageID:   messageID,
		MessageType: schema.MsgReport,
		IntentID:    report.IntentID,
		CommandID:   report.CommandID,
		EventID:     report.EventID,
		Status:      report.CompletionState,
	})
}

// Peer placeholder so a missing identity is explicit rather than an empty required field.
func peerOrUnknown(peer string) string {
	if p := strings.TrimSpace(peer); p != "" {
		return p
	}
	return "unknown"
}
