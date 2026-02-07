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
