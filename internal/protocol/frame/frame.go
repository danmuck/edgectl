package frame

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	FixedHeaderLen uint16 = 32
	FlagHasAuth    uint32 = 0x01
	FlagIsResponse uint32 = 0x02
	FlagIsError    uint32 = 0x04
)

var (
	ErrShortHeader       = errors.New("frame: short fixed header")
	ErrHeaderLenTooSmall = errors.New("frame: header_len smaller than fixed header")
	ErrHeaderLenMismatch = errors.New("frame: auth present but header_len has no auth bytes")
	ErrPayloadTooLarge   = errors.New("frame: payload too large")
	ErrAuthTooLarge      = errors.New("frame: auth too large")
)

// Header is the fixed wire header.
type Header struct {
	Magic       uint32
	Version     uint16
	HeaderLen   uint16
	MessageID   uint64
	MessageType uint32
	Flags       uint32
	PayloadLen  uint64
}

// Frame is one complete wire message.
type Frame struct {
	Header  Header
	Auth    []byte
	Payload []byte
}

// Limits constrains frame decode/encode memory use.
type Limits struct {
	MaxAuthBytes    uint64
	MaxPayloadBytes uint64
}

func DefaultLimits() Limits {
	return Limits{
		MaxAuthBytes:    64 * 1024,
		MaxPayloadBytes: 8 * 1024 * 1024,
	}
}

func ReadFrame(r io.Reader, limits Limits) (Frame, error) {
	var fixed [FixedHeaderLen]byte
	if _, err := io.ReadFull(r, fixed[:]); err != nil {
		if errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, io.EOF) {
			return Frame{}, ErrShortHeader
		}
		return Frame{}, err
	}

	h, err := DecodeHeader(fixed[:])
	if err != nil {
		return Frame{}, err
	}

	if h.HeaderLen < FixedHeaderLen {
		return Frame{}, ErrHeaderLenTooSmall
	}

	authLen := uint64(h.HeaderLen - FixedHeaderLen)
	if h.Flags&FlagHasAuth != 0 && authLen == 0 {
		return Frame{}, ErrHeaderLenMismatch
	}
	if authLen > limits.MaxAuthBytes {
		return Frame{}, ErrAuthTooLarge
	}
	if h.PayloadLen > limits.MaxPayloadBytes {
		return Frame{}, ErrPayloadTooLarge
	}

	auth := make([]byte, authLen)
	if authLen > 0 {
		if _, err := io.ReadFull(r, auth); err != nil {
			return Frame{}, err
		}
	}

	payload := make([]byte, h.PayloadLen)
	if h.PayloadLen > 0 {
		if _, err := io.ReadFull(r, payload); err != nil {
			return Frame{}, err
		}
	}

	return Frame{Header: h, Auth: auth, Payload: payload}, nil
}

func WriteFrame(w io.Writer, f Frame, limits Limits) error {
	authLen := uint64(len(f.Auth))
	payloadLen := uint64(len(f.Payload))
	if authLen > limits.MaxAuthBytes {
		return ErrAuthTooLarge
	}
	if payloadLen > limits.MaxPayloadBytes {
		return ErrPayloadTooLarge
	}

	h := f.Header
	h.HeaderLen = FixedHeaderLen + uint16(authLen)
	h.PayloadLen = payloadLen
	if authLen > 0 {
		h.Flags |= FlagHasAuth
	} else {
		h.Flags &^= FlagHasAuth
	}

	hb := EncodeHeader(h)
	if _, err := w.Write(hb); err != nil {
		return err
	}
	if authLen > 0 {
		if _, err := w.Write(f.Auth); err != nil {
			return err
		}
	}
	if payloadLen > 0 {
		if _, err := w.Write(f.Payload); err != nil {
			return err
		}
	}
	return nil
}

func EncodeHeader(h Header) []byte {
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

func DecodeHeader(b []byte) (Header, error) {
	if len(b) != int(FixedHeaderLen) {
		return Header{}, fmt.Errorf("frame: invalid fixed header length: %d", len(b))
	}
	return Header{
		Magic:       binary.BigEndian.Uint32(b[0:4]),
		Version:     binary.BigEndian.Uint16(b[4:6]),
		HeaderLen:   binary.BigEndian.Uint16(b[6:8]),
		MessageID:   binary.BigEndian.Uint64(b[8:16]),
		MessageType: binary.BigEndian.Uint32(b[16:20]),
		Flags:       binary.BigEndian.Uint32(b[20:24]),
		PayloadLen:  binary.BigEndian.Uint64(b[24:32]),
	}, nil
}
