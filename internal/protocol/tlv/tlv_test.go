package tlv

import (
	"bytes"
	"errors"
	"testing"
)

func TestEncodeDecodeFieldsRoundTripPreservesUnknown(t *testing.T) {
	in := []Field{
		{ID: 1, Type: TypeString, Value: []byte("intent-1")},
		{ID: 9999, Type: TypeBytes, Value: []byte{0xAA, 0xBB}}, // unknown field id
	}
	b := EncodeFields(in)
	out, err := DecodeFields(b)
	if err != nil {
		t.Fatalf("decode fields: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("expected 2 fields, got %d", len(out))
	}
	if out[1].ID != 9999 || out[1].Type != TypeBytes || !bytes.Equal(out[1].Value, []byte{0xAA, 0xBB}) {
		t.Fatalf("unknown field not preserved: %+v", out[1])
	}
}

func TestDecodeFieldsMalformedHeaderIsDeterministic(t *testing.T) {
	_, err := DecodeFields([]byte{1, 2, 3})
	if !errors.Is(err, ErrShortFieldHeader) {
		t.Fatalf("expected ErrShortFieldHeader, got %v", err)
	}
}

func TestDecodeFieldsMalformedLengthIsDeterministic(t *testing.T) {
	// id=1, type=string, len=5, value only 2 bytes
	payload := []byte{0, 1, TypeString, 0, 0, 0, 5, 'a', 'b'}
	_, err := DecodeFields(payload)
	if !errors.Is(err, ErrShortFieldValue) {
		t.Fatalf("expected ErrShortFieldValue, got %v", err)
	}
}
