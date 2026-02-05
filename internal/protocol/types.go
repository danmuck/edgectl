package protocol

const (
	// Magic is the protocol identifier in the fixed header.
	Magic uint32 = 0x45444745 // "EDGE" in ASCII

	// Version is the current protocol version.
	Version uint16 = 1

	// HeaderSize is the fixed header size in bytes for the current version.
	HeaderSize uint16 = 32
)

const (
	FlagHasAuth    uint32 = 0x01
	FlagIsResponse uint32 = 0x02
	FlagIsError    uint32 = 0x04
)

// MessageType identifies the semantic message type.
type MessageType uint32

const (
	MessageIntent MessageType = iota + 1
	MessageCommand
	MessageEvent
	MessageStreamOpen
	MessageStreamData
	MessageStreamClose
	MessageError
)

// FieldType identifies the primitive encoding used in a TLV value.
type FieldType uint8

const (
	FieldUint8 FieldType = iota + 1
	FieldUint16
	FieldUint32
	FieldUint64
	FieldBool
	FieldString
	FieldBytes
)

// Header is the fixed-size message header.
type Header struct {
	Magic       uint32
	Version     uint16
	HeaderLen   uint16
	MessageID   uint64
	MessageType MessageType
	Flags       uint32
	PayloadLen  uint64
}

// Field is a single TLV payload field.
type Field struct {
	ID    uint16
	Type  FieldType
	Value []byte
}

// Message is a fully decoded control-plane message.
type Message struct {
	Header    Header
	AuthBlock []byte
	Fields    []Field
}
