package schema

// Wire error codes from docs/architecture/definitions/errors.toml.
const (
	ErrCodeTransportFailure       uint32 = 1000
	ErrCodeFramingInvalidHeader   uint32 = 1100
	ErrCodeFramingOversize        uint32 = 1101
	ErrCodeTLVDecodeFailure       uint32 = 1200
	ErrCodeSemanticValidation     uint32 = 1300
	ErrCodeRuntimeExecFailure     uint32 = 1400
	ErrCodeInternalError          uint32 = 1500
)
