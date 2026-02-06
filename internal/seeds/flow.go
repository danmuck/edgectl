package seeds

import (
	"bytes"
	"errors"
	"fmt"
	"sync"

	"github.com/danmuck/edgectl/internal/protocol"
)

// Field IDs for the demo intent, command, and event messages.
const (
	fieldIntentName   uint16 = 1
	fieldIntentTarget uint16 = 2

	fieldCommandName   uint16 = 10
	fieldCommandTarget uint16 = 11
	fieldCorrelationID uint16 = 12

	fieldEventName   uint16 = 20
	fieldEventStatus uint16 = 21
)

// Schema definitions for validating intent, command, and event messages.
var (
	intentSchema = protocol.Schema{
		MessageType: protocol.MessageIntent,
		Fields: []protocol.FieldSpec{
			{ID: fieldIntentName, Type: protocol.FieldString, Required: true},
			{ID: fieldIntentTarget, Type: protocol.FieldString, Required: true},
		},
	}
	commandSchema = protocol.Schema{
		MessageType: protocol.MessageCommand,
		Fields: []protocol.FieldSpec{
			{ID: fieldCommandName, Type: protocol.FieldString, Required: true},
			{ID: fieldCommandTarget, Type: protocol.FieldString, Required: true},
			{ID: fieldCorrelationID, Type: protocol.FieldUint64, Required: true},
		},
	}
	eventSchema = protocol.Schema{
		MessageType: protocol.MessageEvent,
		Fields: []protocol.FieldSpec{
			{ID: fieldEventName, Type: protocol.FieldString, Required: true},
			{ID: fieldEventStatus, Type: protocol.FieldString, Required: true},
			{ID: fieldCorrelationID, Type: protocol.FieldUint64, Required: true},
		},
	}
)

// Flow errors returned when prerequisites are missing.
var (
	errNoIntent  = errors.New("flow: no intent available")
	errNoCommand = errors.New("flow: no command available")
)

// FlowSeed demonstrates an intent -> command -> event flow using the protocol.
type FlowSeed struct {
	mu sync.RWMutex

	nextID uint64

	lastIntent      *protocol.Message
	lastCommand     *protocol.Message
	lastEvent       *protocol.Message
	lastCorrelation uint64
}

// FlowStatus summarizes the last observed flow state.
type FlowStatus struct {
	NextMessageID     uint64
	LastIntentID      uint64
	LastCommandID     uint64
	LastEventID       uint64
	LastCorrelationID uint64
}

// FieldShape is a readable representation of a TLV field.
type FieldShape struct {
	ID    uint16
	Type  protocol.FieldType
	Value string
}

// MessageShape is a readable representation of a protocol message.
type MessageShape struct {
	MessageID   uint64
	MessageType protocol.MessageType
	Fields      []FieldShape
}

// FlowSnapshot captures the current flow state and message shapes.
type FlowSnapshot struct {
	Status  FlowStatus
	Intent  *MessageShape
	Command *MessageShape
	Event   *MessageShape
}

// Name returns the seed identifier.
func (s *FlowSeed) Name() string {
	return "flow"
}

// Status returns the current flow status snapshot.
func (s *FlowSeed) Status() (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return FlowStatus{
		NextMessageID:     s.nextID + 1,
		LastIntentID:      messageID(s.lastIntent),
		LastCommandID:     messageID(s.lastCommand),
		LastEventID:       messageID(s.lastEvent),
		LastCorrelationID: s.lastCorrelation,
	}, nil
}

// Snapshot returns a readable snapshot of the current flow state.
func (s *FlowSeed) Snapshot() FlowSnapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return FlowSnapshot{
		Status: FlowStatus{
			NextMessageID:     s.nextID + 1,
			LastIntentID:      messageID(s.lastIntent),
			LastCommandID:     messageID(s.lastCommand),
			LastEventID:       messageID(s.lastEvent),
			LastCorrelationID: s.lastCorrelation,
		},
		Intent:  messageShape(s.lastIntent),
		Command: messageShape(s.lastCommand),
		Event:   messageShape(s.lastEvent),
	}
}

// Actions returns the flow demo actions keyed by name.
func (s *FlowSeed) Actions() map[string]Action {
	return map[string]Action{
		"intent":    s.emitIntent,
		"command":   s.emitCommand,
		"event":     s.emitEvent,
		"flow-demo": s.runFlow,
	}
}

// emitIntent builds and validates an intent message and stores it.
func (s *FlowSeed) emitIntent() (string, error) {
	msg := s.newMessage(protocol.MessageIntent, []protocol.Field{
		protocol.NewFieldString(fieldIntentName, "sync-state"),
		protocol.NewFieldString(fieldIntentTarget, "edge-ctl"),
	})

	decoded, err := roundTrip(msg)
	if err != nil {
		return "", err
	}
	if _, err := protocol.ParseSemantic(decoded, intentSchema); err != nil {
		return "", err
	}

	s.recordIntent(decoded)
	return "intent stored", nil
}

// emitCommand builds and validates a command correlated to the last intent.
func (s *FlowSeed) emitCommand() (string, error) {
	intentID, err := s.intentID()
	if err != nil {
		return "", err
	}

	msg := s.newMessage(protocol.MessageCommand, []protocol.Field{
		protocol.NewFieldString(fieldCommandName, "apply-config"),
		protocol.NewFieldString(fieldCommandTarget, "edge-ctl"),
		protocol.NewFieldUint64(fieldCorrelationID, intentID),
	})

	decoded, err := roundTrip(msg)
	if err != nil {
		return "", err
	}
	if _, err := protocol.ParseSemantic(decoded, commandSchema); err != nil {
		return "", err
	}

	s.recordCommand(decoded, intentID)
	return "command stored", nil
}

// emitEvent builds and validates an event correlated to the last command.
func (s *FlowSeed) emitEvent() (string, error) {
	_, correlationID, err := s.commandID()
	if err != nil {
		return "", err
	}

	msg := s.newMessage(protocol.MessageEvent, []protocol.Field{
		protocol.NewFieldString(fieldEventName, "config-applied"),
		protocol.NewFieldString(fieldEventStatus, "ok"),
		protocol.NewFieldUint64(fieldCorrelationID, correlationID),
	})

	decoded, err := roundTrip(msg)
	if err != nil {
		return "", err
	}
	if _, err := protocol.ParseSemantic(decoded, eventSchema); err != nil {
		return "", err
	}

	s.recordEvent(decoded, correlationID)
	return "event stored", nil
}

// runFlow executes the intent -> command -> event sequence.
func (s *FlowSeed) runFlow() (string, error) {
	if _, err := s.emitIntent(); err != nil {
		return "", err
	}
	if _, err := s.emitCommand(); err != nil {
		return "", err
	}
	if _, err := s.emitEvent(); err != nil {
		return "", err
	}
	return "flow complete", nil
}

// newMessage constructs a message with a fresh ID and provided fields.
func (s *FlowSeed) newMessage(messageType protocol.MessageType, fields []protocol.Field) *protocol.Message {
	return &protocol.Message{
		Header: protocol.Header{
			MessageID:   s.nextMessageID(),
			MessageType: messageType,
		},
		Fields: fields,
	}
}

// nextMessageID increments and returns the next message ID.
func (s *FlowSeed) nextMessageID() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	return s.nextID
}

// recordIntent stores the last intent message.
func (s *FlowSeed) recordIntent(msg *protocol.Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastIntent = msg
}

// recordCommand stores the last command message and correlation ID.
func (s *FlowSeed) recordCommand(msg *protocol.Message, correlationID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastCommand = msg
	s.lastCorrelation = correlationID
}

// recordEvent stores the last event message and correlation ID.
func (s *FlowSeed) recordEvent(msg *protocol.Message, correlationID uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastEvent = msg
	s.lastCorrelation = correlationID
}

// intentID returns the last intent message ID.
func (s *FlowSeed) intentID() (uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastIntent == nil {
		return 0, errNoIntent
	}
	return s.lastIntent.Header.MessageID, nil
}

// commandID returns the last command ID and correlation ID.
func (s *FlowSeed) commandID() (uint64, uint64, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.lastCommand == nil {
		return 0, 0, errNoCommand
	}
	return s.lastCommand.Header.MessageID, s.lastCorrelation, nil
}

// roundTrip encodes and decodes a message to validate wire compatibility.
func roundTrip(msg *protocol.Message) (*protocol.Message, error) {
	var buf bytes.Buffer
	if err := protocol.Encode(&buf, msg); err != nil {
		return nil, err
	}
	return protocol.Decode(bytes.NewReader(buf.Bytes()))
}

// messageID returns the message ID or zero for nil.
func messageID(msg *protocol.Message) uint64 {
	if msg == nil {
		return 0
	}
	return msg.Header.MessageID
}

func messageShape(msg *protocol.Message) *MessageShape {
	if msg == nil {
		return nil
	}
	fields := make([]FieldShape, 0, len(msg.Fields))
	for _, field := range msg.Fields {
		fields = append(fields, FieldShape{
			ID:    field.ID,
			Type:  field.Type,
			Value: fieldValueString(field),
		})
	}
	return &MessageShape{
		MessageID:   msg.Header.MessageID,
		MessageType: msg.Header.MessageType,
		Fields:      fields,
	}
}

func fieldValueString(field protocol.Field) string {
	switch field.Type {
	case protocol.FieldUint8:
		v, err := field.Uint8()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%d", v)
	case protocol.FieldUint16:
		v, err := field.Uint16()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%d", v)
	case protocol.FieldUint32:
		v, err := field.Uint32()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%d", v)
	case protocol.FieldUint64:
		v, err := field.Uint64()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%d", v)
	case protocol.FieldBool:
		v, err := field.Bool()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%t", v)
	case protocol.FieldString:
		v, err := field.String()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%q", v)
	case protocol.FieldBytes:
		v, err := field.Bytes()
		if err != nil {
			return fmt.Sprintf("invalid(%v)", err)
		}
		return fmt.Sprintf("%x", v)
	default:
		return fmt.Sprintf("unknown(%x)", field.Value)
	}
}
