package frame

import (
	"bytes"
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/tlv"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	logs "github.com/danmuck/smplog"
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

// TestWriteFrameRejectsAuthBytesBeyondLimits verifies auth byte upper-bound enforcement.
func TestWriteFrameRejectsAuthBytesBeyondLimits(t *testing.T) {
	testlog.Start(t)

	limits := Limits{
		MaxAuthBytes:    2,
		MaxPayloadBytes: 64,
	}
	in := Frame{
		Header: Header{Magic: ProtocolMagic, Version: ProtocolVersion, MessageID: 44, MessageType: 2},
		Auth:   []byte("abc"),
	}
	logs.Infof("test writing frame message_id=%d auth_len=%d limits=%+v", in.Header.MessageID, len(in.Auth), limits)

	var buf bytes.Buffer
	err := WriteFrame(&buf, in, limits)
	if !errors.Is(err, ErrAuthTooLarge) {
		t.Fatalf("expected ErrAuthTooLarge, got %v", err)
	}
}

// TestWriteFrameRejectsPayloadBeyondLimits verifies payload byte upper-bound enforcement.
func TestWriteFrameRejectsPayloadBytesBeyondLimits(t *testing.T) {
	testlog.Start(t)

	limits := Limits{
		MaxAuthBytes:    64,
		MaxPayloadBytes: 2,
	}
	in := Frame{
		Header:  Header{Magic: ProtocolMagic, Version: ProtocolVersion, MessageID: 45, MessageType: 2},
		Payload: []byte("abc"),
	}
	logs.Infof("test writing frame message_id=%d payload_len=%d limits=%+v", in.Header.MessageID, len(in.Payload), limits)

	var buf bytes.Buffer
	err := WriteFrame(&buf, in, limits)
	if !errors.Is(err, ErrPayloadTooLarge) {
		t.Fatalf("expected ErrPayloadTooLarge, got %v", err)
	}
}

// TestWriteFrameClearsAuthFlagWhenAuthEmpty verifies flag normalization for empty auth blocks.
func TestWriteFrameClearsAuthFlagWhenAuthEmpty(t *testing.T) {
	testlog.Start(t)

	in := Frame{
		Header:  Header{Magic: ProtocolMagic, Version: ProtocolVersion, MessageID: 46, MessageType: 2, Flags: FlagHasAuth},
		Payload: []byte("ok"),
	}
	var buf bytes.Buffer
	if err := WriteFrame(&buf, in, DefaultLimits()); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	out, err := ReadFrame(&buf, DefaultLimits())
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	logs.Infof("test round trip header flags=0x%08X auth_len=%d", out.Header.Flags, len(out.Auth))

	if out.Header.Flags&FlagHasAuth != 0 {
		t.Fatalf("expected auth flag cleared, got flags=0x%08X", out.Header.Flags)
	}
	if len(out.Auth) != 0 {
		t.Fatalf("expected empty auth bytes, got=%d", len(out.Auth))
	}
}
