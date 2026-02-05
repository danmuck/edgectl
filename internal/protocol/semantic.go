package protocol

import "fmt"

// FieldSpec declares a known field within a message type.
type FieldSpec struct {
	ID       uint16
	Type     FieldType
	Required bool
}

// Schema defines required and known fields for a message type.
type Schema struct {
	MessageType MessageType
	Fields      []FieldSpec
}

// Value is a decoded field value.
type Value struct {
	Type   FieldType
	Uint8  uint8
	Uint16 uint16
	Uint32 uint32
	Uint64 uint64
	Bool   bool
	String string
	Bytes  []byte
}

// SemanticMessage is a message with typed field values validated by a schema.
type SemanticMessage struct {
	Header      Header
	AuthBlock   []byte
	MessageType MessageType
	Fields      map[uint16]Value
	Unknown     []Field
}

// ParseSemantic validates msg against schema and returns typed field values.
func ParseSemantic(msg *Message, schema Schema) (*SemanticMessage, error) {
	if msg == nil {
		return nil, ErrInvalidLength
	}
	if msg.Header.MessageType != schema.MessageType {
		return nil, ErrMessageTypeMismatch
	}
	known := make(map[uint16]FieldSpec, len(schema.Fields))
	required := make(map[uint16]struct{})
	for _, spec := range schema.Fields {
		known[spec.ID] = spec
		if spec.Required {
			required[spec.ID] = struct{}{}
		}
	}

	semantic := &SemanticMessage{
		Header:      msg.Header,
		AuthBlock:   msg.AuthBlock,
		MessageType: msg.Header.MessageType,
		Fields:      make(map[uint16]Value),
	}

	for _, field := range msg.Fields {
		spec, ok := known[field.ID]
		if !ok {
			semantic.Unknown = append(semantic.Unknown, field)
			continue
		}
		value, err := decodeValue(field, spec.Type)
		if err != nil {
			return nil, err
		}
		semantic.Fields[field.ID] = value
		delete(required, field.ID)
	}

	if len(required) != 0 {
		for id := range required {
			return nil, MissingFieldError{FieldID: id}
		}
	}

	return semantic, nil
}

// MissingFieldError indicates a required field was not present.
type MissingFieldError struct {
	FieldID uint16
}

func (e MissingFieldError) Error() string {
	return fmt.Sprintf("protocol: missing required field %d", e.FieldID)
}

func decodeValue(field Field, expected FieldType) (Value, error) {
	if field.Type != expected {
		return Value{}, ErrFieldTypeMismatch
	}
	value := Value{Type: field.Type}
	switch field.Type {
	case FieldUint8:
		v, err := field.Uint8()
		if err != nil {
			return Value{}, err
		}
		value.Uint8 = v
	case FieldUint16:
		v, err := field.Uint16()
		if err != nil {
			return Value{}, err
		}
		value.Uint16 = v
	case FieldUint32:
		v, err := field.Uint32()
		if err != nil {
			return Value{}, err
		}
		value.Uint32 = v
	case FieldUint64:
		v, err := field.Uint64()
		if err != nil {
			return Value{}, err
		}
		value.Uint64 = v
	case FieldBool:
		v, err := field.Bool()
		if err != nil {
			return Value{}, err
		}
		value.Bool = v
	case FieldString:
		v, err := field.String()
		if err != nil {
			return Value{}, err
		}
		value.String = v
	case FieldBytes:
		v, err := field.Bytes()
		if err != nil {
			return Value{}, err
		}
		value.Bytes = v
	default:
		return Value{}, ErrFieldTypeMismatch
	}
	return value, nil
}
