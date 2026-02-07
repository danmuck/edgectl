package frame

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	logs "github.com/danmuck/smplog"
)

const (
	FixedHeaderLen  uint16 = 32
	ProtocolMagic   uint32 = 0xEDCE1001
	ProtocolVersion uint16 = 1
	FlagHasAuth     uint32 = 0x01
	FlagIsResponse  uint32 = 0x02
	FlagIsError     uint32 = 0x04
	SupportedFlags  uint32 = FlagHasAuth | FlagIsResponse | FlagIsError
)

var (
	ErrShortHeader        = errors.New("frame: short fixed header")
	ErrHeaderLenTooSmall  = errors.New("frame: header_len smaller than fixed header")
	ErrHeaderLenMismatch  = errors.New("frame: auth present but header_len has no auth bytes")
	ErrPayloadTooLarge    = errors.New("frame: payload too large")
	ErrAuthTooLarge       = errors.New("frame: auth too large")
	ErrUnsupportedMagic   = errors.New("frame: unsupported magic")
	ErrUnsupportedVersion = errors.New("frame: unsupported version")
	ErrUnsupportedFlags   = errors.New("frame: unsupported flags")
)

// Frame fixed-width wire header.
type Header struct {
	Magic       uint32
	Version     uint16
	HeaderLen   uint16
	MessageID   uint64
	MessageType uint32
	Flags       uint32
	PayloadLen  uint64
}

// Frame complete wire message containing header, auth bytes, and payload.
type Frame struct {
	Header  Header
	Auth    []byte
	Payload []byte
}

// Frame decode/encode memory limits.
type Limits struct {
	MaxAuthBytes    uint64
	MaxPayloadBytes uint64
}

// Frame package conservative default size limits for runtime decode/encode.
func DefaultLimits() Limits {
	logs.Debug("frame.DefaultLimits")
	return Limits{
		MaxAuthBytes:    64 * 1024,
		MaxPayloadBytes: 8 * 1024 * 1024,
	}
}

// Frame decoder for one full frame with structural validation.
func ReadFrame(r io.Reader, limits Limits) (Frame, error) {
	logs.Debugf(
		"frame.ReadFrame start max_auth=%d max_payload=%d",
		limits.MaxAuthBytes,
		limits.MaxPayloadBytes,
	)
	var fixed [FixedHeaderLen]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			logs.Err("frame.ReadFrame short fixed header")
			return Frame{}, ErrShortHeader
		}
		logs.Errf("frame.ReadFrame fixed header read err=%v", err)
		return Frame{}, err
	}

	h, err := DecodeHeader(fixed[:])
	if err != nil {
		logs.Errf("frame.ReadFrame decode header err=%v", err)
		return Frame{}, err
	}

	if h.HeaderLen < FixedHeaderLen {
		logs.Errf("frame.ReadFrame invalid header_len=%d", h.HeaderLen)
		return Frame{}, ErrHeaderLenTooSmall
	}

	authLen := uint64(h.HeaderLen - FixedHeaderLen)
	if h.Flags&FlagHasAuth != 0 && authLen == 0 {
		logs.Err("frame.ReadFrame auth flag set without auth bytes")
		return Frame{}, ErrHeaderLenMismatch
	}
	if authLen > limits.MaxAuthBytes {
		logs.Errf("frame.ReadFrame auth too large auth_len=%d", authLen)
		return Frame{}, ErrAuthTooLarge
	}
	if h.PayloadLen > limits.MaxPayloadBytes {
		logs.Errf("frame.ReadFrame payload too large payload_len=%d", h.PayloadLen)
		return Frame{}, ErrPayloadTooLarge
	}

	auth := make([]byte, authLen)
	if authLen > 0 {
		if _, err := io.ReadFull(r, auth); err != nil {
			logs.Errf("frame.ReadFrame auth read err=%v", err)
			return Frame{}, err
		}
	}

	payload := make([]byte, h.PayloadLen)
	if h.PayloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			logs.Errf("frame.ReadFrame payload read err=%v", err)
			return Frame{}, err
		}
	}

	logs.Infof(
		"frame.ReadFrame ok message_id=%d message_type=%d payload_len=%d auth_len=%d",
		h.MessageID,
		h.MessageType,
		h.PayloadLen,
		authLen,
	)
	return Frame{Header: h, Auth: auth, Payload: payload}, nil
}

// Frame encoder for one full frame with structural validation.
func WriteFrame(w io.Writer, f Frame, limits Limits) error {
	authLen := uint64(len(f.Auth))
	payloadLen := uint64(len(f.Payload))
	logs.Debugf(
		"frame.WriteFrame message_id=%d message_type=%d auth_len=%d payload_len=%d",
		f.Header.MessageID,
		f.Header.MessageType,
		authLen,
		payloadLen,
	)
	if authLen > limits.MaxAuthBytes {
		logs.Errf("frame.WriteFrame auth too large auth_len=%d", authLen)
		return ErrAuthTooLarge
	}
	if payloadLen > limits.MaxPayloadBytes {
		logs.Errf("frame.WriteFrame payload too large payload_len=%d", payloadLen)
		return ErrPayloadTooLarge
	}

	h := f.Header
	if h.Magic == 0 {
		h.Magic = ProtocolMagic
	}
	if h.Version == 0 {
		h.Version = ProtocolVersion
	}
	if h.Flags&^SupportedFlags != 0 {
		logs.Errf("frame.WriteFrame unsupported flags=0x%08X", h.Flags)
		return fmt.Errorf("%w: flags=0x%08X", ErrUnsupportedFlags, h.Flags)
	}
	h.HeaderLen = FixedHeaderLen + uint16(authLen)
	h.PayloadLen = payloadLen
	if authLen > 0 {
		h.Flags |= FlagHasAuth
	} else {
		h.Flags &^= FlagHasAuth
	}

	hb := EncodeHeader(h)
	if _, err := w.Write(hb); err != nil {
		logs.Errf("frame.WriteFrame write fixed header err=%v", err)
		return err
	}
	if authLen > 0 {
		if _, err := w.Write(f.Auth); err != nil {
			logs.Errf("frame.WriteFrame write auth err=%v", err)
			return err
		}
	}
	if payloadLen > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			logs.Errf("frame.WriteFrame write payload err=%v", err)
			return err
		}
	}
	logs.Infof(
		"frame.WriteFrame ok message_id=%d message_type=%d payload_len=%d auth_len=%d",
		h.MessageID,
		h.MessageType,
		payloadLen,
		authLen,
	)
	return nil
}

// Frame header serializer for fixed-width protocol header bytes.
func EncodeHeader(h Header) []byte {
	logs.Debugf("frame.EncodeHeader message_id=%d message_type=%d", h.MessageID, h.MessageType)
	buf := make([]byte, FixedHeaderLen)
	binary.BigEndian.PutUint32(buf[0:4], h.Magic)
	binary.BigEndian.PutUint16(buf[4:6], h.Version)
	binary.BigEndian.PutUint16(buf[6:8], h.HeaderLen)
	binary.BigEndian.PutUint64(buf[8:16], h.MessageID)
	binary.BigEndian.PutUint32(buf[16:20], h.MessageType)
	binary.BigEndian.PutUint32(buf[20:24], h.Flags)
	binary.BigEndian.PutUint64(buf[24:32], h.PayloadLen)
	return buf
}

// Frame header parser/validator for fixed-width protocol header bytes.
func DecodeHeader(b []byte) (Header, error) {
	if len(b) != int(FixedHeaderLen) {
		logs.Errf("frame.DecodeHeader invalid length=%d", len(b))
		return Header{}, fmt.Errorf("frame: invalid fixed header length: %d", len(b))
	}
	h := Header{
		Magic:       binary.BigEndian.Uint32(b[0:4]),
		Version:     binary.BigEndian.Uint16(b[4:6]),
		HeaderLen:   binary.BigEndian.Uint16(b[6:8]),
		MessageID:   binary.BigEndian.Uint64(b[8:16]),
		MessageType: binary.BigEndian.Uint32(b[16:20]),
		Flags:       binary.BigEndian.Uint32(b[20:24]),
		PayloadLen:  binary.BigEndian.Uint64(b[24:32]),
	}
	if h.Magic != ProtocolMagic {
		logs.Errf("frame.DecodeHeader unsupported magic=0x%08X", h.Magic)
		return Header{}, fmt.Errorf("%w: magic=0x%08X", ErrUnsupportedMagic, h.Magic)
	}
	if h.Version != ProtocolVersion {
		logs.Errf("frame.DecodeHeader unsupported version=%d", h.Version)
		return Header{}, fmt.Errorf("%w: version=%d", ErrUnsupportedVersion, h.Version)
	}
	if h.Flags&^SupportedFlags != 0 {
		logs.Errf("frame.DecodeHeader unsupported flags=0x%08X", h.Flags)
		return Header{}, fmt.Errorf("%w: flags=0x%08X", ErrUnsupportedFlags, h.Flags)
	}
	logs.Debugf("frame.DecodeHeader ok message_id=%d message_type=%d", h.MessageID, h.MessageType)
	return h, nil
}
