package protocol

import (
	"encoding/binary"
	"errors"
)

// NewFieldUint8 creates a uint8 TLV field.
func NewFieldUint8(id uint16, v uint8) Field {
	return Field{ID: id, Type: FieldUint8, Value: []byte{v}}
}

// NewFieldUint16 creates a uint16 TLV field.
func NewFieldUint16(id uint16, v uint16) Field {
	buf := make([]byte, 2)
	binary.BigEndian.PutUint16(buf, v)
	return Field{ID: id, Type: FieldUint16, Value: buf}
}

// NewFieldUint32 creates a uint32 TLV field.
func NewFieldUint32(id uint16, v uint32) Field {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf, v)
	return Field{ID: id, Type: FieldUint32, Value: buf}
}

// NewFieldUint64 creates a uint64 TLV field.
func NewFieldUint64(id uint16, v uint64) Field {
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, v)
	return Field{ID: id, Type: FieldUint64, Value: buf}
}

// NewFieldBool creates a bool TLV field.
func NewFieldBool(id uint16, v bool) Field {
	b := byte(0)
	if v {
		b = 1
	}
	return Field{ID: id, Type: FieldBool, Value: []byte{b}}
}

// NewFieldString creates a string TLV field.
func NewFieldString(id uint16, v string) Field {
	return Field{ID: id, Type: FieldString, Value: []byte(v)}
}

// NewFieldBytes creates a bytes TLV field.
func NewFieldBytes(id uint16, v []byte) Field {
	buf := make([]byte, len(v))
	copy(buf, v)
	return Field{ID: id, Type: FieldBytes, Value: buf}
}

// Uint8 returns the field value as uint8.
func (f Field) Uint8() (uint8, error) {
	if f.Type != FieldUint8 {
		return 0, ErrFieldTypeMismatch
	}
	if len(f.Value) != 1 {
		return 0, ErrInvalidLength
	}
	return f.Value[0], nil
}

// Uint16 returns the field value as uint16.
func (f Field) Uint16() (uint16, error) {
	if f.Type != FieldUint16 {
		return 0, ErrFieldTypeMismatch
	}
	if len(f.Value) != 2 {
		return 0, ErrInvalidLength
	}
	return binary.BigEndian.Uint16(f.Value), nil
}

// Uint32 returns the field value as uint32.
func (f Field) Uint32() (uint32, error) {
	if f.Type != FieldUint32 {
		return 0, ErrFieldTypeMismatch
	}
	if len(f.Value) != 4 {
		return 0, ErrInvalidLength
	}
	return binary.BigEndian.Uint32(f.Value), nil
}

// Uint64 returns the field value as uint64.
func (f Field) Uint64() (uint64, error) {
	if f.Type != FieldUint64 {
		return 0, ErrFieldTypeMismatch
	}
	if len(f.Value) != 8 {
		return 0, ErrInvalidLength
	}
	return binary.BigEndian.Uint64(f.Value), nil
}

// Bool returns the field value as bool.
func (f Field) Bool() (bool, error) {
	if f.Type != FieldBool {
		return false, ErrFieldTypeMismatch
	}
	if len(f.Value) != 1 {
		return false, ErrInvalidLength
	}
	switch f.Value[0] {
	case 0:
		return false, nil
	case 1:
		return true, nil
	default:
		return false, errors.New("protocol: invalid bool value")
	}
}

// String returns the field value as string.
func (f Field) String() (string, error) {
	if f.Type != FieldString {
		return "", ErrFieldTypeMismatch
	}
	return string(f.Value), nil
}

// Bytes returns the field value as bytes.
func (f Field) Bytes() ([]byte, error) {
	if f.Type != FieldBytes {
		return nil, ErrFieldTypeMismatch
	}
	buf := make([]byte, len(f.Value))
	copy(buf, f.Value)
	return buf, nil
}
