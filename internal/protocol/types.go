package protocol

// Header is the fixed frame header contract skeleton.
type Header struct {
	Magic      uint32
	Version    uint16
	HeaderLen  uint16
	MessageID  uint64
	MessageType uint32
	Flags      uint32
	PayloadLen uint64
}
