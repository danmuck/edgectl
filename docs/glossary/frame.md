# Frame Codec Slice (Go Definitions)

This file defines minimal frame/header codec shapes.
Contract references:

- `../architecture/framing.md`
- `../architecture/definitions/protocol.toml`

## Header Constants

```go
const HeaderSize = 32
```

```go
const (
	FlagHasAuth    uint32 = 0x01
	FlagIsResponse uint32 = 0x02
	FlagIsError    uint32 = 0x04
)
```

## Header and Frame Types

```go
type Header struct {
	Magic      uint32
	Version    uint16
	HeaderLen  uint16
	MessageID  uint64
	MessageType uint32
	Flags      uint32
	PayloadLen uint64
}
```

```go
type Frame struct {
	Header  Header
	Auth    []byte
	Payload []byte
}
```

```go
type Limits struct {
	MaxPayloadBytes uint64
	MaxAuthBytes    uint64
}
```

## Minimal Codec Interfaces

```go
type HeaderCodec interface {
	EncodeHeader(h Header) ([]byte, error)
	DecodeHeader(b []byte) (Header, error)
}
```

```go
type FrameCodec interface {
	ReadFrame(conn SessionConn, limits Limits) (Frame, error)
	WriteFrame(conn SessionConn, f Frame, limits Limits) error
}
```

## Validation Helpers

```go
func ValidateHeader(h Header, expectedMagic uint32, supportedVersion uint16, limits Limits) error
```

```go
func ValidateFrame(f Frame, limits Limits) error
```

## Reader/Writer Primitives

```go
func ReadExact(conn SessionConn, n uint64) ([]byte, error)
```

```go
func WriteAll(conn SessionConn, b []byte) error
```
