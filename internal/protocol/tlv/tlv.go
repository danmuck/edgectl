package tlv

import (
	"encoding/binary"
	"errors"
	"fmt"
)

const HeaderLen = 7

var (
	ErrShortFieldHeader = errors.New("tlv: short field header")
	ErrShortFieldValue  = errors.New("tlv: short field value")
)

// Type IDs from tlv contract.
const (
	TypeU8     uint8 = 1
	TypeU16    uint8 = 2
	TypeU32    uint8 = 3
	TypeU64    uint8 = 4
	TypeBool   uint8 = 5
	TypeString uint8 = 6
	TypeBytes  uint8 = 7
)

// Field is one decoded TLV field.
type Field struct {
	ID    uint16
	Type  uint8
	Value []byte
}

func EncodeField(f Field) []byte {
	buf := make([]byte, HeaderLen+len(f.Value))
	binary.BigEndian.PutUint16(buf[0:2], f.ID)
	buf[2] = f.Type
	binary.BigEndian.PutUint32(buf[3:7], uint32(len(f.Value)))
	copy(buf[7:], f.Value)
	return buf
}

func DecodeFields(payload []byte) ([]Field, error) {
	fields := make([]Field, 0)
	i := 0
	for i < len(payload) {
		if len(payload)-i < HeaderLen {
			return nil, ErrShortFieldHeader
		}
		id := binary.BigEndian.Uint16(payload[i : i+2])
		typeID := payload[i+2]
		l := binary.BigEndian.Uint32(payload[i+3 : i+7])
		i += HeaderLen
		if uint32(len(payload)-i) < l {
			return nil, ErrShortFieldValue
		}
		val := make([]byte, l)
		copy(val, payload[i:i+int(l)])
		i += int(l)
		fields = append(fields, Field{ID: id, Type: typeID, Value: val})
	}
	return fields, nil
}

func EncodeFields(fields []Field) []byte {
	out := make([]byte, 0)
	for _, f := range fields {
		out = append(out, EncodeField(f)...)
	}
	return out
}

func GetField(fields []Field, id uint16) (Field, bool) {
	for _, f := range fields {
		if f.ID == id {
			return f, true
		}
	}
	return Field{}, false
}

func MustType(f Field, expected uint8) error {
	if f.Type != expected {
		return fmt.Errorf("tlv: field %d type mismatch: got %d want %d", f.ID, f.Type, expected)
	}
	return nil
}

func U32FromBytes(b []byte) (uint32, error) {
	if len(b) != 4 {
		return 0, fmt.Errorf("tlv: invalid u32 length: %d", len(b))
	}
	return binary.BigEndian.Uint32(b), nil
}
