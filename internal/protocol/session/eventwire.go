package session

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"strings"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
)

// Event is the wire-transport event shape sent Ghost->Mirage.
type Event struct {
	EventID     string
	CommandID   string
	IntentID    string
	GhostID     string
	SeedID      string
	Outcome     string
	TimestampMS uint64
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.EventID) == "" {
		return fmt.Errorf("event missing event_id")
	}
	if strings.TrimSpace(e.CommandID) == "" {
		return fmt.Errorf("event missing command_id")
	}
	if strings.TrimSpace(e.IntentID) == "" {
		return fmt.Errorf("event missing intent_id")
	}
	if strings.TrimSpace(e.GhostID) == "" {
		return fmt.Errorf("event missing ghost_id")
	}
	if strings.TrimSpace(e.SeedID) == "" {
		return fmt.Errorf("event missing seed_id")
	}
	if strings.TrimSpace(e.Outcome) == "" {
		return fmt.Errorf("event missing outcome")
	}
	return nil
}

// EventAck is the wire-transport acknowledgment shape sent Mirage->Ghost.
type EventAck struct {
	EventID     string
	CommandID   string
	GhostID     string
	AckStatus   string
	AckCode     uint32
	TimestampMS uint64
}

func (a EventAck) Validate() error {
	if strings.TrimSpace(a.EventID) == "" {
		return fmt.Errorf("event.ack missing event_id")
	}
	if strings.TrimSpace(a.CommandID) == "" {
		return fmt.Errorf("event.ack missing command_id")
	}
	if strings.TrimSpace(a.GhostID) == "" {
		return fmt.Errorf("event.ack missing ghost_id")
	}
	if strings.TrimSpace(a.AckStatus) == "" {
		return fmt.Errorf("event.ack missing ack_status")
	}
	if a.TimestampMS == 0 {
		return fmt.Errorf("event.ack missing timestamp_ms")
	}
	return nil
}

func EncodeEventFrame(messageID uint64, event Event) ([]byte, error) {
	if err := event.Validate(); err != nil {
		return nil, err
	}
	fields := []tlv.Field{
		{ID: schema.FieldEventID, Type: tlv.TypeString, Value: []byte(event.EventID)},
		{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte(event.CommandID)},
		{ID: schema.FieldIntentID, Type: tlv.TypeString, Value: []byte(event.IntentID)},
		{ID: schema.FieldGhostID, Type: tlv.TypeString, Value: []byte(event.GhostID)},
		{ID: schema.FieldSeedID, Type: tlv.TypeString, Value: []byte(event.SeedID)},
		{ID: schema.FieldOutcome, Type: tlv.TypeString, Value: []byte(event.Outcome)},
	}
	if event.TimestampMS != 0 {
		fields = append(fields, tlv.Field{ID: schema.FieldTimestampMS, Type: tlv.TypeU64, Value: putU64(event.TimestampMS)})
	}
	if err := schema.Validate(schema.MsgEvent, fields); err != nil {
		return nil, err
	}
	payload := tlv.EncodeFields(fields)
	var buf bytes.Buffer
	err := frame.WriteFrame(&buf, frame.Frame{
		Header: frame.Header{
			MessageID:   messageID,
			MessageType: schema.MsgEvent,
		},
		Payload: payload,
	}, frame.DefaultLimits())
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeEventFrame(f frame.Frame) (Event, error) {
	fields, err := tlv.DecodeFields(f.Payload)
	if err != nil {
		return Event{}, err
	}
	if err := schema.Validate(schema.MsgEvent, fields); err != nil {
		return Event{}, err
	}
	event := Event{
		EventID:   getRequiredString(fields, schema.FieldEventID),
		CommandID: getRequiredString(fields, schema.FieldCommandID),
		IntentID:  getRequiredString(fields, schema.FieldIntentID),
		GhostID:   getRequiredString(fields, schema.FieldGhostID),
		SeedID:    getRequiredString(fields, schema.FieldSeedID),
		Outcome:   getRequiredString(fields, schema.FieldOutcome),
	}
	if tsField, ok := tlv.GetField(fields, schema.FieldTimestampMS); ok {
		ts, err := u64FromBytes(tsField.Value)
		if err != nil {
			return Event{}, err
		}
		event.TimestampMS = ts
	}
	return event, nil
}

func EncodeEventAckFrame(messageID uint64, ack EventAck) ([]byte, error) {
	if err := ack.Validate(); err != nil {
		return nil, err
	}
	fields := []tlv.Field{
		{ID: schema.FieldEventID, Type: tlv.TypeString, Value: []byte(ack.EventID)},
		{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte(ack.CommandID)},
		{ID: schema.FieldGhostID, Type: tlv.TypeString, Value: []byte(ack.GhostID)},
		{ID: schema.FieldAckStatus, Type: tlv.TypeString, Value: []byte(ack.AckStatus)},
		{ID: schema.FieldAckCode, Type: tlv.TypeU32, Value: putU32(ack.AckCode)},
		{ID: schema.FieldTimestampMS, Type: tlv.TypeU64, Value: putU64(ack.TimestampMS)},
	}
	if err := schema.Validate(schema.MsgEventAck, fields); err != nil {
		return nil, err
	}
	payload := tlv.EncodeFields(fields)
	var buf bytes.Buffer
	err := frame.WriteFrame(&buf, frame.Frame{
		Header: frame.Header{
			MessageID:   messageID,
			MessageType: schema.MsgEventAck,
			Flags:       frame.FlagIsResponse,
		},
		Payload: payload,
	}, frame.DefaultLimits())
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func DecodeEventAckFrame(f frame.Frame) (EventAck, error) {
	fields, err := tlv.DecodeFields(f.Payload)
	if err != nil {
		return EventAck{}, err
	}
	if err := schema.Validate(schema.MsgEventAck, fields); err != nil {
		return EventAck{}, err
	}
	ack := EventAck{
		EventID:     getRequiredString(fields, schema.FieldEventID),
		CommandID:   getRequiredString(fields, schema.FieldCommandID),
		GhostID:     getRequiredString(fields, schema.FieldGhostID),
		AckStatus:   getRequiredString(fields, schema.FieldAckStatus),
		TimestampMS: getRequiredU64(fields, schema.FieldTimestampMS),
	}
	if ackField, ok := tlv.GetField(fields, schema.FieldAckCode); ok {
		v, err := tlv.U32FromBytes(ackField.Value)
		if err != nil {
			return EventAck{}, err
		}
		ack.AckCode = v
	}
	return ack, nil
}

// ReadFrame reads one framed message from the stream.
func ReadFrame(r io.Reader, limits frame.Limits) (frame.Frame, error) {
	return frame.ReadFrame(r, limits)
}

func getRequiredString(fields []tlv.Field, id uint16) string {
	f, _ := tlv.GetField(fields, id)
	return string(f.Value)
}

func getRequiredU64(fields []tlv.Field, id uint16) uint64 {
	f, _ := tlv.GetField(fields, id)
	v, _ := u64FromBytes(f.Value)
	return v
}

func putU32(v uint32) []byte {
	out := make([]byte, 4)
	binary.BigEndian.PutUint32(out, v)
	return out
}

func putU64(v uint64) []byte {
	out := make([]byte, 8)
	binary.BigEndian.PutUint64(out, v)
	return out
}

func u64FromBytes(b []byte) (uint64, error) {
	if len(b) != 8 {
		return 0, fmt.Errorf("session: invalid u64 length: %d", len(b))
	}
	return binary.BigEndian.Uint64(b), nil
}
