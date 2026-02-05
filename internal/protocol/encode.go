package protocol

import (
	"encoding/binary"
	"io"
)

const fieldHeaderSize = 2 + 1 + 4

// Encode writes msg to w using the protocol wire format.
func Encode(w io.Writer, msg *Message) error {
	if msg == nil {
		return ErrInvalidLength
	}
	payloadLen, err := payloadLength(msg.Fields)
	if err != nil {
		return err
	}
	if msg.Header.Flags&FlagHasAuth == 0 && len(msg.AuthBlock) > 0 {
		return ErrAuthFlagMismatch
	}
	if len(msg.AuthBlock) > int(^uint16(0)) {
		return ErrAuthTooLarge
	}

	head := msg.Header
	head.Magic = Magic
	head.Version = Version
	head.HeaderLen = HeaderSize
	head.PayloadLen = payloadLen

	headerBytes := encodeHeader(head)
	if _, err := w.Write(headerBytes); err != nil {
		return err
	}

	if head.Flags&FlagHasAuth != 0 {
		if err := writeAuthBlock(w, msg.AuthBlock); err != nil {
			return err
		}
	}

	for _, field := range msg.Fields {
		if err := writeField(w, field); err != nil {
			return err
		}
	}

	return nil
}

func payloadLength(fields []Field) (uint64, error) {
	var total uint64
	for _, field := range fields {
		if len(field.Value) > int(^uint32(0)) {
			return 0, ErrInvalidLength
		}
		total += uint64(fieldHeaderSize + len(field.Value))
	}
	return total, nil
}

func encodeHeader(h Header) []byte {
	buf := make([]byte, HeaderSize)
	binary.BigEndian.PutUint32(buf[0:4], h.Magic)
	binary.BigEndian.PutUint16(buf[4:6], h.Version)
	binary.BigEndian.PutUint16(buf[6:8], h.HeaderLen)
	binary.BigEndian.PutUint64(buf[8:16], h.MessageID)
	binary.BigEndian.PutUint32(buf[16:20], uint32(h.MessageType))
	binary.BigEndian.PutUint32(buf[20:24], h.Flags)
	binary.BigEndian.PutUint64(buf[24:32], h.PayloadLen)
	return buf
}

func writeAuthBlock(w io.Writer, auth []byte) error {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, uint16(len(auth)))
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if len(auth) == 0 {
		return nil
	}
	_, err := w.Write(auth)
	return err
}

func writeField(w io.Writer, field Field) error {
	if len(field.Value) > int(^uint32(0)) {
		return ErrInvalidLength
	}
	buf := make([]byte, fieldHeaderSize)
	binary.BigEndian.PutUint16(buf[0:2], field.ID)
	buf[2] = byte(field.Type)
	binary.BigEndian.PutUint32(buf[3:7], uint32(len(field.Value)))
	if _, err := w.Write(buf); err != nil {
		return err
	}
	if len(field.Value) == 0 {
		return nil
	}
	_, err := w.Write(field.Value)
	return err
}
