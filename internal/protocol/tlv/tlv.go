package tlv

import (
	"encoding/binary"
	"errors"
	"fmt"

	logs "github.com/danmuck/smplog"
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

// TLV decoded field tuple.
type Field struct {
	ID    uint16
	Type  uint8
	Value []byte
}

// TLV serializer for one field header and value bytes.
func EncodeField(f Field) []byte {
	logs.Debugf("tlv.EncodeField id=%d type=%d len=%d", f.ID, f.Type, len(f.Value))
	buf := make([]byte, HeaderLen+len(f.Value))
	binary.BigEndian.PutUint16(buf[0:2], f.ID)
	buf[2] = f.Type
	binary.BigEndian.PutUint32(buf[3:7], uint32(len(f.Value)))
	copy(buf[7:], f.Value)
	return buf
}

// TLV parser for payload bytes into ordered fields.
func DecodeFields(payload []byte) ([]Field, error) {
	logs.Debugf("tlv.DecodeFields start len=%d", len(payload))
	fields := make([]Field, 0)
	i := 0
	for i < len(payload) {
		if len(payload)-i < HeaderLen {
			logs.Errf("tlv.DecodeFields short header offset=%d", i)
			return nil, ErrShortFieldHeader
		}
		id := binary.BigEndian.Uint16(payload[i : i+2])
		typeID := payload[i+2]
		l := binary.BigEndian.Uint32(payload[i+3 : i+7])
		i += HeaderLen
		if uint32(len(payload)-i) < l {
			logs.Errf("tlv.DecodeFields short value id=%d expected=%d remaining=%d", id, l, len(payload)-i)
			return nil, ErrShortFieldValue
		}
		val := make([]byte, l)
		copy(val, payload[i:i+int(l)])
		i += int(l)
		fields = append(fields, Field{ID: id, Type: typeID, Value: val})
	}
	logs.Infof("tlv.DecodeFields ok fields=%d", len(fields))
	return fields, nil
}

// TLV serializer for an ordered set of fields.
func EncodeFields(fields []Field) []byte {
	logs.Debugf("tlv.EncodeFields count=%d", len(fields))
	out := make([]byte, 0)
	for _, f := range fields {
		out = append(out, EncodeField(f)...)
	}
	logs.Debugf("tlv.EncodeFields bytes=%d", len(out))
	return out
}

// TLV field lookup returning the first field with matching id.
func GetField(fields []Field, id uint16) (Field, bool) {
	logs.Debugf("tlv.GetField id=%d count=%d", id, len(fields))
	for _, f := range fields {
		if f.ID == id {
			logs.Debugf("tlv.GetField found id=%d type=%d", f.ID, f.Type)
			return f, true
		}
	}
	logs.Debugf("tlv.GetField missing id=%d", id)
	return Field{}, false
}

// TLV type-check helper for field type id validation.
func MustType(f Field, expected uint8) error {
	if f.Type != expected {
		logs.Errf("tlv.MustType mismatch id=%d got=%d want=%d", f.ID, f.Type, expected)
		return fmt.Errorf("tlv: field %d type mismatch: got %d want %d", f.ID, f.Type, expected)
	}
	logs.Debugf("tlv.MustType ok id=%d type=%d", f.ID, f.Type)
	return nil
}

// TLV helper decoding a big-endian uint32 from fixed-length bytes.
func U32FromBytes(b []byte) (uint32, error) {
	if len(b) != 4 {
		logs.Errf("tlv.U32FromBytes invalid len=%d", len(b))
		return 0, fmt.Errorf("tlv: invalid u32 length: %d", len(b))
	}
	v := binary.BigEndian.Uint32(b)
	logs.Debugf("tlv.U32FromBytes value=%d", v)
	return v, nil
}
