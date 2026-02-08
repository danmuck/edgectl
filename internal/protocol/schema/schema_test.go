package schema

import (
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestValidateIssueRequiredFields(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldIntentID, Type: tlv.TypeString, Value: []byte("intent-1")},
		{ID: FieldActor, Type: tlv.TypeString, Value: []byte("user:dan")},
		{ID: FieldTargetScope, Type: tlv.TypeString, Value: []byte("ghost:*")},
		{ID: FieldObjective, Type: tlv.TypeString, Value: []byte("restart mongodb")},
	}
	if err := Validate(MsgIssue, fields); err != nil {
		t.Fatalf("validate issue: %v", err)
	}
}

func TestValidateUnknownFieldsIgnored(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldIntentID, Type: tlv.TypeString, Value: []byte("intent-1")},
		{ID: FieldActor, Type: tlv.TypeString, Value: []byte("user:dan")},
		{ID: FieldTargetScope, Type: tlv.TypeString, Value: []byte("ghost:*")},
		{ID: FieldObjective, Type: tlv.TypeString, Value: []byte("restart mongodb")},
		{ID: 9999, Type: tlv.TypeBytes, Value: []byte{0x01}},
	}
	if err := Validate(MsgIssue, fields); err != nil {
		t.Fatalf("validate with unknown field: %v", err)
	}
}

func TestValidateMissingRequiredDeterministic(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{{ID: FieldIntentID, Type: tlv.TypeString, Value: []byte("intent-1")}}
	err := Validate(MsgIssue, fields)
	if err == nil {
		t.Fatalf("expected error")
	}
	ve, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.FieldID != FieldActor || ve.Reason != "missing required field" {
		t.Fatalf("unexpected validation error: %+v", ve)
	}
}

func TestValidateTypeMismatchDeterministic(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldIntentID, Type: tlv.TypeString, Value: []byte("intent-1")},
		{ID: FieldActor, Type: tlv.TypeString, Value: []byte("user:dan")},
		{ID: FieldTargetScope, Type: tlv.TypeString, Value: []byte("ghost:*")},
		{ID: FieldObjective, Type: tlv.TypeU32, Value: []byte{0, 0, 0, 1}},
	}
	err := Validate(MsgIssue, fields)
	if err == nil {
		t.Fatalf("expected error")
	}
	ve, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.FieldID != FieldObjective || ve.Reason != "type mismatch" {
		t.Fatalf("unexpected validation error: %+v", ve)
	}
}

func TestValidateEventAckRequiredFields(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldEventID, Type: tlv.TypeString, Value: []byte("evt.1")},
		{ID: FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.1")},
		{ID: FieldGhostID, Type: tlv.TypeString, Value: []byte("ghost.1")},
		{ID: FieldAckStatus, Type: tlv.TypeString, Value: []byte("accepted")},
		{ID: FieldTimestampMS, Type: tlv.TypeU64, Value: []byte{0, 0, 0, 0, 0, 0, 0, 1}},
	}
	if err := Validate(MsgEventAck, fields); err != nil {
		t.Fatalf("validate event.ack: %v", err)
	}
}

func TestValidateEventAckMissingTimestampDeterministic(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldEventID, Type: tlv.TypeString, Value: []byte("evt.1")},
		{ID: FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.1")},
		{ID: FieldGhostID, Type: tlv.TypeString, Value: []byte("ghost.1")},
		{ID: FieldAckStatus, Type: tlv.TypeString, Value: []byte("accepted")},
	}
	err := Validate(MsgEventAck, fields)
	if err == nil {
		t.Fatalf("expected error")
	}
	ve, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.FieldID != FieldTimestampMS || ve.Reason != "missing required field" {
		t.Fatalf("unexpected validation error: %+v", ve)
	}
}

func TestValidateSeedExecuteUsesCanonicalFieldIDs(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldExecutionID, Type: tlv.TypeString, Value: []byte("exec.1")},
		{ID: FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.1")},
		{ID: FieldSeedID, Type: tlv.TypeString, Value: []byte("seed.flow")},
		{ID: FieldSeedExecuteOperation, Type: tlv.TypeString, Value: []byte("status")},
		{ID: FieldSeedExecuteArgs, Type: tlv.TypeBytes, Value: []byte("{}")},
	}
	if err := Validate(MsgSeedExecute, fields); err != nil {
		t.Fatalf("validate seed.execute: %v", err)
	}
}

func TestValidateSeedExecuteLegacyFieldIDsRejected(t *testing.T) {
	testlog.Start(t)
	fields := []tlv.Field{
		{ID: FieldExecutionID, Type: tlv.TypeString, Value: []byte("exec.1")},
		{ID: FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.1")},
		{ID: FieldSeedID, Type: tlv.TypeString, Value: []byte("seed.flow")},
		{ID: FieldOperation, Type: tlv.TypeString, Value: []byte("status")},
		{ID: FieldArgs, Type: tlv.TypeBytes, Value: []byte("{}")},
	}
	err := Validate(MsgSeedExecute, fields)
	if err == nil {
		t.Fatalf("expected error")
	}
	ve, ok := err.(ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	if ve.FieldID != FieldSeedExecuteOperation || ve.Reason != "missing required field" {
		t.Fatalf("unexpected validation error: %+v", ve)
	}
}
