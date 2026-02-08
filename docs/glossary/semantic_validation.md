# Semantic Validation Slice (Stub)

Status: `stub`

Contract references:

- `../architecture/definitions/protocol.toml`
- `../architecture/definitions/tlv.toml`

## Planned Go Definitions

```go
type ValidationError struct {
	MessageType uint32
	Field       string
	Reason      string
}
```

```go
func (e ValidationError) Error() string
```

```go
func ValidateByMessageType(messageType uint32, fields []Field) error
```

```go
func RequireField(fields []Field, id uint16, typeID uint8) error
```
