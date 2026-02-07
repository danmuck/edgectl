# TLV Codec Slice (Stub)

Status: `stub`

Contract references:

- `../architecture/definitions/tlv.toml`
- `tlv.md`

## Planned Go Definitions

```go
type Field struct {
	ID     uint16
	Type   uint8
	Length uint32
	Value  []byte
}
```

```go
func EncodeField(f Field) ([]byte, error)
```

```go
func DecodeFields(payload []byte) ([]Field, error)
```

```go
func GetString(fields []Field, id uint16) (string, bool, error)
```

```go
func GetU64(fields []Field, id uint16) (uint64, bool, error)
```
