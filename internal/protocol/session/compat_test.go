package session

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"reflect"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Every protocol error class maps to its errors.toml wire code, and all codes fall in 1000-1500.
func TestErrorCodeTaxonomy(t *testing.T) {
	testlog.Start(t)
	cases := []struct {
		name string
		err  error
		want uint32
	}{
		{"nil", nil, 0},
		{"unsupported version", fmt.Errorf("%w: version=2", frame.ErrUnsupportedVersion), schema.ErrCodeFramingInvalidHeader},
		{"unsupported magic", frame.ErrUnsupportedMagic, schema.ErrCodeFramingInvalidHeader},
		{"unsupported flags", frame.ErrUnsupportedFlags, schema.ErrCodeFramingInvalidHeader},
		{"short header", frame.ErrShortHeader, schema.ErrCodeFramingInvalidHeader},
		{"header len too small", frame.ErrHeaderLenTooSmall, schema.ErrCodeFramingInvalidHeader},
		{"payload too large", frame.ErrPayloadTooLarge, schema.ErrCodeFramingOversize},
		{"auth too large", frame.ErrAuthTooLarge, schema.ErrCodeFramingOversize},
		{"control too large", ErrControlMessageTooLarge, schema.ErrCodeFramingOversize},
		{"tlv short header", tlv.ErrShortFieldHeader, schema.ErrCodeTLVDecodeFailure},
		{"tlv short value", tlv.ErrShortFieldValue, schema.ErrCodeTLVDecodeFailure},
		{"tlv invalid value", fmt.Errorf("%w: u64", tlv.ErrInvalidFieldValue), schema.ErrCodeTLVDecodeFailure},
		{"schema validation", schema.ValidationError{MessageType: schema.MsgEvent, FieldID: 4, Reason: "missing"}, schema.ErrCodeSemanticValidation},
		{"invalid registration", fmt.Errorf("%w: missing ghost_id", ErrInvalidRegistration), schema.ErrCodeSemanticValidation},
		{"eof", io.EOF, schema.ErrCodeTransportFailure},
		{"deadline", context.DeadlineExceeded, schema.ErrCodeTransportFailure},
		{"net op", &net.OpError{Op: "dial", Err: errors.New("refused")}, schema.ErrCodeTransportFailure},
		{"unclassified", errors.New("boom"), schema.ErrCodeInternalError},
	}
	for _, tc := range cases {
		got := ErrorCode(tc.err)
		if got != tc.want {
			t.Errorf("%s: ErrorCode = %d, want %d", tc.name, got, tc.want)
		}
		if tc.err != nil && (got < 1000 || got > 1500) {
			t.Errorf("%s: code %d outside canonical 1000-1500", tc.name, got)
		}
	}
}

// Raw frame bytes with an explicit header so tests can set fields the encoder would refuse.
func rawFrame(version uint16, flags uint32, messageType uint32, fields []tlv.Field) []byte {
	payload := tlv.EncodeFields(fields)
	h := frame.Header{
		Magic:       frame.ProtocolMagic,
		Version:     version,
		HeaderLen:   frame.FixedHeaderLen,
		MessageID:   1,
		MessageType: messageType,
		Flags:       flags,
		PayloadLen:  uint64(len(payload)),
	}
	return append(frame.EncodeHeader(h), payload...)
}

func statusCommandFields() []tlv.Field {
	return []tlv.Field{
		{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte("cmd.compat.1")},
		{ID: schema.FieldIntentID, Type: tlv.TypeString, Value: []byte("intent.compat")},
		{ID: schema.FieldGhostID, Type: tlv.TypeString, Value: []byte("ghost.alpha")},
		{ID: schema.FieldSeedSelector, Type: tlv.TypeString, Value: []byte("seed.flow")},
		{ID: schema.FieldOperation, Type: tlv.TypeString, Value: []byte("status")},
	}
}

func TestFrameVersionMismatchRejectedAsInvalidHeader(t *testing.T) {
	testlog.Start(t)
	raw := rawFrame(frame.ProtocolVersion+1, 0, schema.MsgCommand, statusCommandFields())
	_, err := ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
	if !errors.Is(err, frame.ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
	if code := ErrorCode(err); code != schema.ErrCodeFramingInvalidHeader {
		t.Fatalf("error code = %d, want %d", code, schema.ErrCodeFramingInvalidHeader)
	}
}

func TestFrameUnknownFlagBitsRejectedAsInvalidHeader(t *testing.T) {
	testlog.Start(t)
	for _, bit := range []uint32{0x08, 0x10, 0x80000000} {
		raw := rawFrame(frame.ProtocolVersion, bit, schema.MsgCommand, statusCommandFields())
		_, err := ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
		if !errors.Is(err, frame.ErrUnsupportedFlags) {
			t.Fatalf("flag 0x%08X: expected ErrUnsupportedFlags, got %v", bit, err)
		}
		if code := ErrorCode(err); code != schema.ErrCodeFramingInvalidHeader {
			t.Fatalf("flag 0x%08X: error code = %d, want %d", bit, code, schema.ErrCodeFramingInvalidHeader)
		}
	}
}

func TestFrameOversizePayloadRejectedAsOversize(t *testing.T) {
	testlog.Start(t)
	raw := rawFrame(frame.ProtocolVersion, 0, schema.MsgCommand, statusCommandFields())
	binary.BigEndian.PutUint64(raw[24:32], frame.DefaultLimits().MaxPayloadBytes+1)
	_, err := ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
	if code := ErrorCode(err); code != schema.ErrCodeFramingOversize {
		t.Fatalf("error code = %d (%v), want %d", code, err, schema.ErrCodeFramingOversize)
	}
}

// Unknown fields placed before and after known fields are preserved raw and never change semantics.
func TestUnknownCommandFieldsPreservedAndInert(t *testing.T) {
	testlog.Start(t)
	unknownBefore := tlv.Field{ID: 9000, Type: tlv.TypeString, Value: []byte("operation=stop")}
	unknownAfter := tlv.Field{ID: 9001, Type: tlv.TypeBytes, Value: []byte{0xde, 0xad}}
	fields := append([]tlv.Field{unknownBefore}, statusCommandFields()...)
	fields = append(fields, unknownAfter)

	fr, err := ReadFrame(bytes.NewReader(rawFrame(frame.ProtocolVersion, 0, schema.MsgCommand, fields)), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	cmd, err := DecodeCommandFrame(fr)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cmd.Operation != "status" || cmd.SeedSelector != "seed.flow" || cmd.GhostID != "ghost.alpha" {
		t.Fatalf("unknown fields altered semantics: %+v", cmd)
	}
	if !reflect.DeepEqual(cmd.UnknownFields, []tlv.Field{unknownBefore, unknownAfter}) {
		t.Fatalf("unknown fields not preserved: %+v", cmd.UnknownFields)
	}

	// Re-encode keeps the unknown fields byte-for-byte.
	reencoded, err := EncodeCommandFrame(2, cmd)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	fr2, err := ReadFrame(bytes.NewReader(reencoded), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read re-encoded: %v", err)
	}
	again, err := DecodeCommandFrame(fr2)
	if err != nil {
		t.Fatalf("decode re-encoded: %v", err)
	}
	if !reflect.DeepEqual(again.UnknownFields, cmd.UnknownFields) {
		t.Fatalf("re-encode lost unknown fields: %+v", again.UnknownFields)
	}
}

// A caller cannot inject a contract-defined field through UnknownFields.
func TestUnknownFieldsCannotCarryKnownIDs(t *testing.T) {
	testlog.Start(t)
	cmd := Command{
		CommandID: "cmd.compat.2", IntentID: "intent.compat", GhostID: "ghost.alpha",
		SeedSelector: "seed.flow", Operation: "status",
		UnknownFields: []tlv.Field{{ID: schema.FieldOperation, Type: tlv.TypeString, Value: []byte("stop")}},
	}
	raw, err := EncodeCommandFrame(3, cmd)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	fr, err := ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	fields, err := tlv.DecodeFields(fr.Payload)
	if err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	count := 0
	for _, f := range fields {
		if f.ID == schema.FieldOperation {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("operation field emitted %d times, want 1", count)
	}
	decoded, _ := DecodeCommandFrame(fr)
	if decoded.Operation != "status" || len(decoded.UnknownFields) != 0 {
		t.Fatalf("known-ID injection leaked: %+v", decoded)
	}
}

// Event, event.ack, and report also preserve unknown fields across decode and re-encode.
func TestUnknownFieldsRoundTripForEventAckReport(t *testing.T) {
	testlog.Start(t)
	unknown := []tlv.Field{{ID: 9100, Type: tlv.TypeString, Value: []byte("future")}}

	evRaw, err := EncodeEventFrame(1, Event{
		EventID: "evt.c", CommandID: "cmd.c", IntentID: "intent.c", GhostID: "ghost.alpha",
		SeedID: "seed.flow", Outcome: "success", UnknownFields: unknown,
	})
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	evFr, _ := ReadFrame(bytes.NewReader(evRaw), frame.DefaultLimits())
	ev, err := DecodeEventFrame(evFr)
	if err != nil || !reflect.DeepEqual(ev.UnknownFields, unknown) || ev.Outcome != "success" {
		t.Fatalf("event round trip: %+v err=%v", ev, err)
	}

	ackRaw, err := EncodeEventAckFrame(2, EventAck{
		EventID: "evt.c", CommandID: "cmd.c", GhostID: "ghost.alpha", AckStatus: AckStatusAccepted,
		TimestampMS: 1, UnknownFields: unknown,
	})
	if err != nil {
		t.Fatalf("encode ack: %v", err)
	}
	ackFr, _ := ReadFrame(bytes.NewReader(ackRaw), frame.DefaultLimits())
	ack, err := DecodeEventAckFrame(ackFr)
	if err != nil || !reflect.DeepEqual(ack.UnknownFields, unknown) || ack.AckStatus != AckStatusAccepted {
		t.Fatalf("ack round trip: %+v err=%v", ack, err)
	}

	repRaw, err := EncodeReportFrame(3, Report{
		IntentID: "intent.c", Phase: "complete", Summary: "s", CompletionState: "satisfied", UnknownFields: unknown,
	})
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	repFr, _ := ReadFrame(bytes.NewReader(repRaw), frame.DefaultLimits())
	rep, err := DecodeReportFrame(repFr)
	if err != nil || !reflect.DeepEqual(rep.UnknownFields, unknown) || rep.CompletionState != "satisfied" {
		t.Fatalf("report round trip: %+v err=%v", rep, err)
	}
}

// Malformed TLV lengths and invalid typed values map to tlv_decode_failure.
func TestMalformedTLVMapsToDecodeFailure(t *testing.T) {
	testlog.Start(t)
	raw := rawFrame(frame.ProtocolVersion, 0, schema.MsgCommand, statusCommandFields())
	// Truncate the payload by one byte while keeping the declared payload_len consistent.
	raw = raw[:len(raw)-1]
	binary.BigEndian.PutUint64(raw[24:32], uint64(len(raw))-uint64(frame.FixedHeaderLen))
	fr, err := ReadFrame(bytes.NewReader(raw), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	_, err = DecodeCommandFrame(fr)
	if code := ErrorCode(err); code != schema.ErrCodeTLVDecodeFailure {
		t.Fatalf("truncated TLV code = %d (%v), want %d", code, err, schema.ErrCodeTLVDecodeFailure)
	}

	badArgs := append(statusCommandFields(), tlv.Field{ID: schema.FieldArgs, Type: tlv.TypeBytes, Value: []byte("{not json")})
	fr, err = ReadFrame(bytes.NewReader(rawFrame(frame.ProtocolVersion, 0, schema.MsgCommand, badArgs)), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	_, err = DecodeCommandFrame(fr)
	if code := ErrorCode(err); code != schema.ErrCodeTLVDecodeFailure {
		t.Fatalf("bad args code = %d (%v), want %d", code, err, schema.ErrCodeTLVDecodeFailure)
	}

	missing := statusCommandFields()[:3]
	fr, _ = ReadFrame(bytes.NewReader(rawFrame(frame.ProtocolVersion, 0, schema.MsgCommand, missing)), frame.DefaultLimits())
	_, err = DecodeCommandFrame(fr)
	if code := ErrorCode(err); code != schema.ErrCodeSemanticValidation {
		t.Fatalf("missing field code = %d (%v), want %d", code, err, schema.ErrCodeSemanticValidation)
	}
}
