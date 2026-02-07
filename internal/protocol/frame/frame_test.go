package frame

import (
	"bytes"
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestReadWriteFrameRoundTrip(t *testing.T) {
	testlog.Start(t)
	payload := tlv.EncodeFields([]tlv.Field{{ID: 1, Type: tlv.TypeString, Value: []byte("intent-1")}})
	in := Frame{
		Header:  Header{Magic: ProtocolMagic, Version: ProtocolVersion, MessageID: 42, MessageType: 1},
		Auth:    []byte("auth"),
		Payload: payload,
	}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in, DefaultLimits()); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	out, err := ReadFrame(&buf, DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	if out.Header.Magic != in.Header.Magic || out.Header.MessageType != in.Header.MessageType || out.Header.MessageID != in.Header.MessageID {
		t.Fatalf("header mismatch: got=%+v want=%+v", out.Header, in.Header)
	}
	if string(out.Auth) != "auth" {
		t.Fatalf("auth mismatch: %q", string(out.Auth))
	}
	if !bytes.Equal(out.Payload, payload) {
		t.Fatalf("payload mismatch")
	}
}

func TestReadFrameMalformedHeaderIsDeterministic(t *testing.T) {
	testlog.Start(t)
	_, err := ReadFrame(bytes.NewReader([]byte{1, 2, 3}), DefaultLimits())
	if !errors.Is(err, ErrShortHeader) {
		t.Fatalf("expected ErrShortHeader, got %v", err)
	}
}

func TestReadFrameHeaderLenTooSmall(t *testing.T) {
	testlog.Start(t)
	h := Header{Magic: ProtocolMagic, Version: ProtocolVersion, HeaderLen: 8, MessageID: 1, MessageType: 1, PayloadLen: 0}
	buf := EncodeHeader(h)
	_, err := ReadFrame(bytes.NewReader(buf), DefaultLimits())
	if !errors.Is(err, ErrHeaderLenTooSmall) {
		t.Fatalf("expected ErrHeaderLenTooSmall, got %v", err)
	}
}

func TestReadFrameAuthFlagWithoutAuthBytes(t *testing.T) {
	testlog.Start(t)
	h := Header{
		Magic:       ProtocolMagic,
		Version:     ProtocolVersion,
		HeaderLen:   FixedHeaderLen,
		MessageID:   1,
		MessageType: 1,
		Flags:       FlagHasAuth,
		PayloadLen:  0,
	}
	buf := EncodeHeader(h)
	_, err := ReadFrame(bytes.NewReader(buf), DefaultLimits())
	if !errors.Is(err, ErrHeaderLenMismatch) {
		t.Fatalf("expected ErrHeaderLenMismatch, got %v", err)
	}
}

func TestDecodeHeaderRejectsUnsupportedMagic(t *testing.T) {
	testlog.Start(t)
	h := Header{
		Magic:       ProtocolMagic + 1,
		Version:     ProtocolVersion,
		HeaderLen:   FixedHeaderLen,
		MessageID:   1,
		MessageType: 1,
		Flags:       0,
		PayloadLen:  0,
	}
	_, err := DecodeHeader(EncodeHeader(h))
	if !errors.Is(err, ErrUnsupportedMagic) {
		t.Fatalf("expected ErrUnsupportedMagic, got %v", err)
	}
}

func TestDecodeHeaderRejectsUnsupportedVersion(t *testing.T) {
	testlog.Start(t)
	h := Header{
		Magic:       ProtocolMagic,
		Version:     ProtocolVersion + 1,
		HeaderLen:   FixedHeaderLen,
		MessageID:   1,
		MessageType: 1,
		Flags:       0,
		PayloadLen:  0,
	}
	_, err := DecodeHeader(EncodeHeader(h))
	if !errors.Is(err, ErrUnsupportedVersion) {
		t.Fatalf("expected ErrUnsupportedVersion, got %v", err)
	}
}

func TestDecodeHeaderRejectsUnsupportedFlags(t *testing.T) {
	testlog.Start(t)
	h := Header{
		Magic:       ProtocolMagic,
		Version:     ProtocolVersion,
		HeaderLen:   FixedHeaderLen,
		MessageID:   1,
		MessageType: 1,
		Flags:       FlagIsResponse | 0x08,
		PayloadLen:  0,
	}
	_, err := DecodeHeader(EncodeHeader(h))
	if !errors.Is(err, ErrUnsupportedFlags) {
		t.Fatalf("expected ErrUnsupportedFlags, got %v", err)
	}
}
