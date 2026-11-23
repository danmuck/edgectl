package session

import (
	"context"
	"errors"
	"io"
	"net"
	"os"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
)

// ErrorCode maps a protocol-layer error to its canonical wire code (errors.toml [wire_codes]).
// Classes: framing 1100/1101, tlv_decode 1200, semantic 1300, transport 1000; anything
// unrecognized is internal_error 1500. Returns 0 for a nil error. Runtime packages map their own
// execution errors (1400) and fall back to this for protocol errors.
func ErrorCode(err error) uint32 {
	if err == nil {
		return 0
	}
	switch {
	case errors.Is(err, frame.ErrPayloadTooLarge),
		errors.Is(err, frame.ErrAuthTooLarge),
		errors.Is(err, ErrControlMessageTooLarge):
		return schema.ErrCodeFramingOversize
	case errors.Is(err, frame.ErrShortHeader),
		errors.Is(err, frame.ErrHeaderLenTooSmall),
		errors.Is(err, frame.ErrHeaderLenMismatch),
		errors.Is(err, frame.ErrUnsupportedMagic),
		errors.Is(err, frame.ErrUnsupportedVersion),
		errors.Is(err, frame.ErrUnsupportedFlags):
		return schema.ErrCodeFramingInvalidHeader
	case errors.Is(err, tlv.ErrShortFieldHeader),
		errors.Is(err, tlv.ErrShortFieldValue),
		errors.Is(err, tlv.ErrInvalidFieldValue):
		return schema.ErrCodeTLVDecodeFailure
	case isSchemaValidation(err),
		errors.Is(err, ErrInvalidRegistration),
		errors.Is(err, ErrInvalidRegistrationAck):
		return schema.ErrCodeSemanticValidation
	case isTransport(err):
		return schema.ErrCodeTransportFailure
	}
	return schema.ErrCodeInternalError
}

// Schema validation errors are returned by value.
func isSchemaValidation(err error) bool {
	var v schema.ValidationError
	return errors.As(err, &v)
}

// Connection, timeout, and stream-closure failures.
func isTransport(err error) bool {
	if errors.Is(err, io.EOF) ||
		errors.Is(err, io.ErrUnexpectedEOF) ||
		errors.Is(err, net.ErrClosed) ||
		errors.Is(err, os.ErrDeadlineExceeded) ||
		errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr)
}
