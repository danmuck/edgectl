package protocol

import (
	"encoding/binary"
	"io"
)

// Decode reads a single message from r using the protocol wire format.
func Decode(r io.Reader) (*Message, error) {
	headerBytes := make([]byte, HeaderSize)
	if _, err := io.ReadFull(r, headerBytes); err != nil {
		return nil, ErrTruncated
	}

	head, err := parseHeader(headerBytes)
	if err != nil {
		return nil, err
	}

	if head.PayloadLen > uint64(int(^uint(0)>>1)) {
		return nil, ErrPayloadTooLarge
	}

	msg := &Message{Header: head}

	if head.Flags&FlagHasAuth != 0 {
		auth, err := readAuthBlock(r)
		if err != nil {
			return nil, err
		}
		msg.AuthBlock = auth
	}

	payloadLen := int(head.PayloadLen)
	if payloadLen == 0 {
		return msg, nil
	}

	payload := make([]byte, payloadLen)
	if _, err := io.ReadFull(r, payload); err != nil {
		return nil, ErrTruncated
	}

	fields, err := parseFields(payload)
	if err != nil {
		return nil, err
	}
	msg.Fields = fields
	return msg, nil
}

func parseHeader(buf []byte) (Header, error) {
	if len(buf) != int(HeaderSize) {
		return Header{}, ErrTruncated
	}
	h := Header{
		Magic:       binary.BigEndian.Uint32(buf[0:4]),
		Version:     binary.BigEndian.Uint16(buf[4:6]),
		HeaderLen:   binary.BigEndian.Uint16(buf[6:8]),
		MessageID:   binary.BigEndian.Uint64(buf[8:16]),
		MessageType: MessageType(binary.BigEndian.Uint32(buf[16:20])),
		Flags:       binary.BigEndian.Uint32(buf[20:24]),
		PayloadLen:  binary.BigEndian.Uint64(buf[24:32]),
	}
	if h.Magic != Magic {
		return Header{}, ErrInvalidMagic
	}
	if h.Version != Version {
		return Header{}, ErrUnsupportedVersion
	}
	if h.HeaderLen != HeaderSize {
		return Header{}, ErrInvalidHeaderLen
	}
	return h, nil
}

func readAuthBlock(r io.Reader) ([]byte, error) {
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		return nil, ErrTruncated
	}
	authLen := int(binary.BigEndian.Uint16(lenBuf))
	if authLen == 0 {
		return nil, nil
	}
	buf := make([]byte, authLen)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, ErrTruncated
	}
	return buf, nil
}

func parseFields(payload []byte) ([]Field, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	fields := make([]Field, 0, 4)
	for offset := 0; offset < len(payload); {
		remaining := len(payload) - offset
		if remaining < fieldHeaderSize {
			return nil, ErrTruncated
		}
		id := binary.BigEndian.Uint16(payload[offset : offset+2])
		ft := FieldType(payload[offset+2])
		length := binary.BigEndian.Uint32(payload[offset+3 : offset+7])
		offset += fieldHeaderSize
		if length > uint32(len(payload)-offset) {
			return nil, ErrInvalidLength
		}
		if length == 0 {
			fields = append(fields, Field{ID: id, Type: ft})
			continue
		}
		end := offset + int(length)
		value := make([]byte, length)
		copy(value, payload[offset:end])
		fields = append(fields, Field{ID: id, Type: ft, Value: value})
		offset = end
	}
	return fields, nil
}
