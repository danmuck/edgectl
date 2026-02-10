package session

import (
	"bytes"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
)

// encodeFrameForMessage validates fields and encodes one framed TLV message.
func encodeFrameForMessage(
	messageID uint64,
	messageType uint32,
	flags uint32,
	fields []tlv.Field,
) ([]byte, error) {
	return encodeFrameForMessageWithAuth(messageID, messageType, flags, nil, fields)
}

// encodeFrameForMessageWithAuth validates fields and encodes one framed TLV message with optional auth bytes.
func encodeFrameForMessageWithAuth(
	messageID uint64,
	messageType uint32,
	flags uint32,
	auth []byte,
	fields []tlv.Field,
) ([]byte, error) {
	if err := schema.Validate(messageType, fields); err != nil {
		return nil, err
	}

	authCopy := append([]byte{}, auth...)
	var buf bytes.Buffer
	err := frame.WriteFrame(
		&buf,
		frame.Frame{
			Header: frame.Header{
				MessageID:   messageID,
				MessageType: messageType,
				Flags:       flags,
			},
			Auth:    authCopy,
			Payload: tlv.EncodeFields(fields),
		},
		frame.DefaultLimits(),
	)
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
