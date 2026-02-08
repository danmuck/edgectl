package session

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/tlv"
)

// Session wire command payload sent from Mirage to Ghost.
type Command struct {
	CommandID    string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
}

// Session command validator for required payload fields.
func (c Command) Validate() error {
	if strings.TrimSpace(c.CommandID) == "" {
		return fmt.Errorf("command missing command_id")
	}
	if strings.TrimSpace(c.IntentID) == "" {
		return fmt.Errorf("command missing intent_id")
	}
	if strings.TrimSpace(c.GhostID) == "" {
		return fmt.Errorf("command missing ghost_id")
	}
	if strings.TrimSpace(c.SeedSelector) == "" {
		return fmt.Errorf("command missing seed_selector")
	}
	if strings.TrimSpace(c.Operation) == "" {
		return fmt.Errorf("command missing operation")
	}
	return nil
}

// Session wire report payload sent from Mirage to user boundary.
type Report struct {
	IntentID        string
	Phase           string
	Summary         string
	CompletionState string
	CommandID       string
	ExecutionID     string
	EventID         string
	Outcome         string
	TimestampMS     uint64
}

// Session report validator for required payload fields.
func (r Report) Validate() error {
	if strings.TrimSpace(r.IntentID) == "" {
		return fmt.Errorf("report missing intent_id")
	}
	if strings.TrimSpace(r.Phase) == "" {
		return fmt.Errorf("report missing phase")
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("report missing summary")
	}
	if strings.TrimSpace(r.CompletionState) == "" {
		return fmt.Errorf("report missing completion_state")
	}
	return nil
}

// Session encoder for command envelope into framed protocol message bytes.
func EncodeCommandFrame(messageID uint64, command Command) ([]byte, error) {
	if err := command.Validate(); err != nil {
		return nil, err
	}
	fields := []tlv.Field{
		{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte(command.CommandID)},
		{ID: schema.FieldIntentID, Type: tlv.TypeString, Value: []byte(command.IntentID)},
		{ID: schema.FieldGhostID, Type: tlv.TypeString, Value: []byte(command.GhostID)},
		{ID: schema.FieldSeedSelector, Type: tlv.TypeString, Value: []byte(command.SeedSelector)},
		{ID: schema.FieldOperation, Type: tlv.TypeString, Value: []byte(command.Operation)},
	}
	if len(command.Args) > 0 {
		argsPayload, err := json.Marshal(command.Args)
		if err != nil {
			return nil, err
		}
		fields = append(fields, tlv.Field{ID: schema.FieldArgs, Type: tlv.TypeBytes, Value: argsPayload})
	}
	if err := schema.Validate(schema.MsgCommand, fields); err != nil {
		return nil, err
	}
	payload := tlv.EncodeFields(fields)
	var buf bytes.Buffer
	err := frame.WriteFrame(&buf, frame.Frame{
		Header: frame.Header{
			MessageID:   messageID,
			MessageType: schema.MsgCommand,
		},
		Payload: payload,
	}, frame.DefaultLimits())
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Session decoder for one command frame payload with schema validation.
func DecodeCommandFrame(f frame.Frame) (Command, error) {
	fields, err := tlv.DecodeFields(f.Payload)
	if err != nil {
		return Command{}, err
	}
	if err := schema.Validate(schema.MsgCommand, fields); err != nil {
		return Command{}, err
	}
	command := Command{
		CommandID:    getRequiredString(fields, schema.FieldCommandID),
		IntentID:     getRequiredString(fields, schema.FieldIntentID),
		GhostID:      getRequiredString(fields, schema.FieldGhostID),
		SeedSelector: getRequiredString(fields, schema.FieldSeedSelector),
		Operation:    getRequiredString(fields, schema.FieldOperation),
		Args:         map[string]string{},
	}
	if argsField, ok := tlv.GetField(fields, schema.FieldArgs); ok {
		var args map[string]string
		if err := json.Unmarshal(argsField.Value, &args); err != nil {
			return Command{}, err
		}
		command.Args = args
	}
	return command, nil
}

// Session encoder for report envelope into framed protocol message bytes.
func EncodeReportFrame(messageID uint64, report Report) ([]byte, error) {
	if err := report.Validate(); err != nil {
		return nil, err
	}
	fields := []tlv.Field{
		{ID: schema.FieldIntentID, Type: tlv.TypeString, Value: []byte(report.IntentID)},
		{ID: schema.FieldPhase, Type: tlv.TypeString, Value: []byte(report.Phase)},
		{ID: schema.FieldSummary, Type: tlv.TypeString, Value: []byte(report.Summary)},
		{ID: schema.FieldCompletionState, Type: tlv.TypeString, Value: []byte(report.CompletionState)},
	}
	if v := strings.TrimSpace(report.CommandID); v != "" {
		fields = append(fields, tlv.Field{ID: schema.FieldCommandID, Type: tlv.TypeString, Value: []byte(v)})
	}
	if v := strings.TrimSpace(report.ExecutionID); v != "" {
		fields = append(fields, tlv.Field{ID: schema.FieldExecutionID, Type: tlv.TypeString, Value: []byte(v)})
	}
	if v := strings.TrimSpace(report.EventID); v != "" {
		fields = append(fields, tlv.Field{ID: schema.FieldEventID, Type: tlv.TypeString, Value: []byte(v)})
	}
	if v := strings.TrimSpace(report.Outcome); v != "" {
		fields = append(fields, tlv.Field{ID: schema.FieldOutcome, Type: tlv.TypeString, Value: []byte(v)})
	}
	if report.TimestampMS != 0 {
		fields = append(fields, tlv.Field{ID: schema.FieldTimestampMS, Type: tlv.TypeU64, Value: putU64(report.TimestampMS)})
	}
	if err := schema.Validate(schema.MsgReport, fields); err != nil {
		return nil, err
	}
	payload := tlv.EncodeFields(fields)
	var buf bytes.Buffer
	err := frame.WriteFrame(&buf, frame.Frame{
		Header: frame.Header{
			MessageID:   messageID,
			MessageType: schema.MsgReport,
		},
		Payload: payload,
	}, frame.DefaultLimits())
	if err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Session decoder for one report frame payload with schema validation.
func DecodeReportFrame(f frame.Frame) (Report, error) {
	fields, err := tlv.DecodeFields(f.Payload)
	if err != nil {
		return Report{}, err
	}
	if err := schema.Validate(schema.MsgReport, fields); err != nil {
		return Report{}, err
	}
	report := Report{
		IntentID:        getRequiredString(fields, schema.FieldIntentID),
		Phase:           getRequiredString(fields, schema.FieldPhase),
		Summary:         getRequiredString(fields, schema.FieldSummary),
		CompletionState: getRequiredString(fields, schema.FieldCompletionState),
		CommandID:       getOptionalString(fields, schema.FieldCommandID),
		ExecutionID:     getOptionalString(fields, schema.FieldExecutionID),
		EventID:         getOptionalString(fields, schema.FieldEventID),
		Outcome:         getOptionalString(fields, schema.FieldOutcome),
	}
	if tsField, ok := tlv.GetField(fields, schema.FieldTimestampMS); ok {
		ts, err := u64FromBytes(tsField.Value)
		if err != nil {
			return Report{}, err
		}
		report.TimestampMS = ts
	}
	return report, nil
}

// Session helper returning optional string field value if present.
func getOptionalString(fields []tlv.Field, id uint16) string {
	f, ok := tlv.GetField(fields, id)
	if !ok {
		return ""
	}
	return string(f.Value)
}
