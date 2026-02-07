package schema

import (
	"fmt"

	"github.com/danmuck/edgectl/internal/protocol/tlv"
	logs "github.com/danmuck/smplog"
)

// Message type IDs from tlv contract.
const (
	MsgIssue       uint32 = 1
	MsgCommand     uint32 = 2
	MsgSeedExecute uint32 = 3
	MsgSeedResult  uint32 = 4
	MsgEvent       uint32 = 5
	MsgReport      uint32 = 6
	MsgError       uint32 = 7
)

// Field IDs from tlv contract.
const (
	FieldIntentID    uint16 = 1
	FieldCommandID   uint16 = 2
	FieldExecutionID uint16 = 3
	FieldEventID     uint16 = 4
	FieldPhase       uint16 = 5

	FieldActor       uint16 = 100
	FieldTargetScope uint16 = 101
	FieldObjective   uint16 = 102

	FieldGhostID      uint16 = 200
	FieldSeedSelector uint16 = 201
	FieldOperation    uint16 = 202
	FieldArgs         uint16 = 203

	FieldSeedID uint16 = 300

	FieldStatus   uint16 = 400
	FieldStdout   uint16 = 401
	FieldStderr   uint16 = 402
	FieldExitCode uint16 = 403

	FieldOutcome uint16 = 500

	FieldSummary         uint16 = 600
	FieldCompletionState uint16 = 601
)

type Requirement struct {
	ID   uint16
	Type uint8
}

type ValidationError struct {
	MessageType uint32
	FieldID     uint16
	Reason      string
}

func (e ValidationError) Error() string {
	if e.FieldID == 0 {
		return fmt.Sprintf("schema: message_type=%d: %s", e.MessageType, e.Reason)
	}
	return fmt.Sprintf("schema: message_type=%d field=%d: %s", e.MessageType, e.FieldID, e.Reason)
}

var requirements = map[uint32][]Requirement{
	MsgIssue: {
		{FieldIntentID, tlv.TypeString},
		{FieldActor, tlv.TypeString},
		{FieldTargetScope, tlv.TypeString},
		{FieldObjective, tlv.TypeString},
	},
	MsgCommand: {
		{FieldCommandID, tlv.TypeString},
		{FieldIntentID, tlv.TypeString},
		{FieldGhostID, tlv.TypeString},
		{FieldSeedSelector, tlv.TypeString},
		{FieldOperation, tlv.TypeString},
	},
	MsgSeedExecute: {
		{FieldExecutionID, tlv.TypeString},
		{FieldCommandID, tlv.TypeString},
		{FieldSeedID, tlv.TypeString},
		{FieldOperation, tlv.TypeString},
		{FieldArgs, tlv.TypeBytes},
	},
	MsgSeedResult: {
		{FieldExecutionID, tlv.TypeString},
		{FieldSeedID, tlv.TypeString},
		{FieldStatus, tlv.TypeString},
		{FieldStdout, tlv.TypeBytes},
		{FieldStderr, tlv.TypeBytes},
		{FieldExitCode, tlv.TypeU32},
	},
	MsgEvent: {
		{FieldEventID, tlv.TypeString},
		{FieldCommandID, tlv.TypeString},
		{FieldIntentID, tlv.TypeString},
		{FieldGhostID, tlv.TypeString},
		{FieldSeedID, tlv.TypeString},
		{FieldOutcome, tlv.TypeString},
	},
	MsgReport: {
		{FieldIntentID, tlv.TypeString},
		{FieldPhase, tlv.TypeString},
		{FieldSummary, tlv.TypeString},
		{FieldCompletionState, tlv.TypeString},
	},
}

// Validate enforces required fields and required field types for a message type.
// Unknown fields are ignored by design.
func Validate(messageType uint32, fields []tlv.Field) error {
	logs.Debugf("schema.Validate message_type=%d fields=%d", messageType, len(fields))
	reqs, ok := requirements[messageType]
	if !ok {
		logs.Errf("schema.Validate unknown message_type=%d", messageType)
		return ValidationError{MessageType: messageType, Reason: "unknown message_type"}
	}
	for _, req := range reqs {
		f, found := tlv.GetField(fields, req.ID)
		if !found {
			logs.Errf(
				"schema.Validate missing field message_type=%d field_id=%d",
				messageType,
				req.ID,
			)
			return ValidationError{MessageType: messageType, FieldID: req.ID, Reason: "missing required field"}
		}
		if f.Type != req.Type {
			logs.Errf(
				"schema.Validate type mismatch message_type=%d field_id=%d got=%d want=%d",
				messageType,
				req.ID,
				f.Type,
				req.Type,
			)
			return ValidationError{MessageType: messageType, FieldID: req.ID, Reason: "type mismatch"}
		}
	}
	logs.Infof("schema.Validate ok message_type=%d", messageType)
	return nil
}
