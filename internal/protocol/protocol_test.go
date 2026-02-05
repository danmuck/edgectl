package protocol

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
)

func TestRoundTripEncodeDecode(t *testing.T) {
	msg := &Message{
		Header: Header{
			MessageID:   42,
			MessageType: MessageCommand,
			Flags:       FlagHasAuth,
		},
		AuthBlock: []byte{0xaa, 0xbb},
		Fields: []Field{
			NewFieldUint16(1, 99),
			NewFieldString(2, "hello"),
			NewFieldBytes(99, []byte{0x01, 0x02}),
		},
	}

	var buf bytes.Buffer
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("encode: %v", err)
	}

	decoded, err := Decode(bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}

	var buf2 bytes.Buffer
	if err := Encode(&buf2, decoded); err != nil {
		t.Fatalf("re-encode: %v", err)
	}

	if !bytes.Equal(buf.Bytes(), buf2.Bytes()) {
		t.Fatalf("round-trip mismatch")
	}
}

func TestDecodeInvalidMagic(t *testing.T) {
	payload := buildFieldPayload(NewFieldUint8(1, 7))
	head := headerBytes(uint64(len(payload)), 0)
	head[0] = 0
	head[1] = 0
	head[2] = 0
	head[3] = 0

	buf := append(head, payload...)
	_, err := Decode(bytes.NewReader(buf))
	if !errors.Is(err, ErrInvalidMagic) {
		t.Fatalf("expected ErrInvalidMagic, got %v", err)
	}
}

func TestDecodeTruncatedPayload(t *testing.T) {
	msg := &Message{
		Header: Header{MessageID: 1, MessageType: MessageEvent},
		Fields: []Field{NewFieldString(1, "abc")},
	}
	var buf bytes.Buffer
	if err := Encode(&buf, msg); err != nil {
		t.Fatalf("encode: %v", err)
	}

	b := buf.Bytes()
	if len(b) < 2 {
		t.Fatalf("buffer too small")
	}
	b = b[:len(b)-2]
	_, err := Decode(bytes.NewReader(b))
	if !errors.Is(err, ErrTruncated) {
		t.Fatalf("expected ErrTruncated, got %v", err)
	}
}

func TestDecodeInvalidFieldLength(t *testing.T) {
	payload := make([]byte, fieldHeaderSize+1)
	binary.BigEndian.PutUint16(payload[0:2], 1)
	payload[2] = byte(FieldBytes)
	binary.BigEndian.PutUint32(payload[3:7], 5)
	payload[7] = 0xff

	head := headerBytes(uint64(len(payload)), 0)
	buf := append(head, payload...)
	_, err := Decode(bytes.NewReader(buf))
	if !errors.Is(err, ErrInvalidLength) {
		t.Fatalf("expected ErrInvalidLength, got %v", err)
	}
}

func TestSemanticUnknownFieldsIgnored(t *testing.T) {
	msg := &Message{
		Header: Header{MessageID: 1, MessageType: MessageIntent},
		Fields: []Field{
			NewFieldString(1, "intent"),
			NewFieldUint32(99, 123),
		},
	}

	schema := Schema{
		MessageType: MessageIntent,
		Fields: []FieldSpec{
			{ID: 1, Type: FieldString, Required: true},
		},
	}

	parsed, err := ParseSemantic(msg, schema)
	if err != nil {
		t.Fatalf("parse semantic: %v", err)
	}
	if _, ok := parsed.Fields[1]; !ok {
		t.Fatalf("expected known field")
	}
	if len(parsed.Unknown) != 1 {
		t.Fatalf("expected 1 unknown field, got %d", len(parsed.Unknown))
	}
}

func TestSemanticMissingField(t *testing.T) {
	msg := &Message{
		Header: Header{MessageID: 1, MessageType: MessageIntent},
		Fields: []Field{},
	}
	schema := Schema{
		MessageType: MessageIntent,
		Fields: []FieldSpec{
			{ID: 1, Type: FieldString, Required: true},
		},
	}

	_, err := ParseSemantic(msg, schema)
	if err == nil {
		t.Fatalf("expected error")
	}
	var missing MissingFieldError
	if !errors.As(err, &missing) {
		t.Fatalf("expected MissingFieldError, got %v", err)
	}
}

func headerBytes(payloadLen uint64, flags uint32) []byte {
	head := Header{
		Magic:       Magic,
		Version:     Version,
		HeaderLen:   HeaderSize,
		MessageID:   1,
		MessageType: MessageCommand,
		Flags:       flags,
		PayloadLen:  payloadLen,
	}
	return encodeHeader(head)
}

func buildFieldPayload(field Field) []byte {
	buf := make([]byte, 0, fieldHeaderSize+len(field.Value))
	header := make([]byte, fieldHeaderSize)
	binary.BigEndian.PutUint16(header[0:2], field.ID)
	header[2] = byte(field.Type)
	binary.BigEndian.PutUint32(header[3:7], uint32(len(field.Value)))
	buf = append(buf, header...)
	buf = append(buf, field.Value...)
	return buf
}
