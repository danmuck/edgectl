package protocol

import "errors"

var (
	ErrInvalidMagic        = errors.New("protocol: invalid magic")
	ErrUnsupportedVersion  = errors.New("protocol: unsupported version")
	ErrInvalidHeaderLen    = errors.New("protocol: invalid header length")
	ErrPayloadTooLarge     = errors.New("protocol: payload too large")
	ErrAuthTooLarge        = errors.New("protocol: auth block too large")
	ErrTruncated           = errors.New("protocol: truncated data")
	ErrInvalidLength       = errors.New("protocol: invalid length")
	ErrAuthFlagMismatch    = errors.New("protocol: auth flag mismatch")
	ErrFieldTypeMismatch   = errors.New("protocol: field type mismatch")
	ErrMessageTypeMismatch = errors.New("protocol: message type mismatch")
)
