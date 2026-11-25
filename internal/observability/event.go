package observability

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/danmuck/edgectl/internal/activity"

	"github.com/danmuck/edgectl/internal/protocol/schema"
	logs "github.com/danmuck/smplog"
)

// Direction values for LogEvent.Direction, relative to the emitting component.
const (
	DirectionInbound  = "inbound"
	DirectionOutbound = "outbound"
)

// Status values for custody transitions that are not a terminal outcome or command state.
const (
	StatusAccepted     = "accepted"
	StatusDispatched   = "dispatched"
	StatusRetry        = "retry"
	StatusRetryStopped = "retry_stopped"
	StatusReplayed     = "replayed"
	StatusDuplicate    = "duplicate"
	StatusRejected     = "rejected"
)

// ErrMissingLogFields reports a LogEvent lacking observability.toml required fields.
var ErrMissingLogFields = errors.New("observability: missing required log fields")

// LogEvent is one structured custody-chain log record (observability.toml [required_log_fields]).
// Required: component, peer, direction, trace_id, request_id, message_id, message_type, status,
// timestamp_ms. The remaining ID fields are optional correlation keys.
type LogEvent struct {
	Component   string
	Peer        string
	Direction   string
	TraceID     string
	RequestID   string
	MessageID   uint64
	MessageType uint32
	IntentID    string
	CommandID   string
	ExecutionID string
	EventID     string
	AckStatus   string
	Status      string
	Error       string
	TimestampMS uint64
}

// MissingFields lists required fields that are empty or zero, in contract order.
func (e LogEvent) MissingFields() []string {
	var missing []string
	check := func(name string, empty bool) {
		if empty {
			missing = append(missing, name)
		}
	}
	check("component", strings.TrimSpace(e.Component) == "")
	check("peer", strings.TrimSpace(e.Peer) == "")
	check("direction", strings.TrimSpace(e.Direction) == "")
	check("trace_id", strings.TrimSpace(e.TraceID) == "")
	check("request_id", strings.TrimSpace(e.RequestID) == "")
	check("message_id", e.MessageID == 0)
	check("message_type", e.MessageType == 0)
	check("status", strings.TrimSpace(e.Status) == "")
	check("timestamp_ms", e.TimestampMS == 0)
	return missing
}

// Validate returns ErrMissingLogFields naming every absent required field.
func (e LogEvent) Validate() error {
	if missing := e.MissingFields(); len(missing) > 0 {
		return fmt.Errorf("%w: %s", ErrMissingLogFields, strings.Join(missing, ","))
	}
	return nil
}

// Line renders the event as deterministic key=value pairs: required fields first in contract
// order, then optional fields that are set, then error.
func (e LogEvent) Line() string {
	var b strings.Builder
	kv := func(k, v string) {
		if b.Len() > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%s=%q", k, v)
	}
	kv("component", e.Component)
	kv("peer", e.Peer)
	kv("direction", e.Direction)
	kv("trace_id", e.TraceID)
	kv("request_id", e.RequestID)
	fmt.Fprintf(&b, " message_id=%d message_type=%d", e.MessageID, e.MessageType)
	kv("status", e.Status)
	fmt.Fprintf(&b, " timestamp_ms=%d", e.TimestampMS)
	for _, opt := range [][2]string{
		{"intent_id", e.IntentID},
		{"command_id", e.CommandID},
		{"execution_id", e.ExecutionID},
		{"event_id", e.EventID},
		{"ack_status", e.AckStatus},
		{"error", e.Error},
	} {
		if strings.TrimSpace(opt[1]) != "" {
			kv(opt[0], opt[1])
		}
	}
	return b.String()
}

// Key/value form of the event with the same keys Line writes; unset optional fields are omitted.
func (e LogEvent) fields() map[string]string {
	out := map[string]string{
		"component":    e.Component,
		"peer":         e.Peer,
		"direction":    e.Direction,
		"trace_id":     e.TraceID,
		"request_id":   e.RequestID,
		"message_id":   strconv.FormatUint(e.MessageID, 10),
		"message_type": strconv.FormatUint(uint64(e.MessageType), 10),
		"status":       e.Status,
	}
	for _, opt := range [][2]string{
		{"intent_id", e.IntentID},
		{"command_id", e.CommandID},
		{"execution_id", e.ExecutionID},
		{"event_id", e.EventID},
		{"ack_status", e.AckStatus},
		{"error", e.Error},
	} {
		if strings.TrimSpace(opt[1]) != "" {
			out[opt[0]] = opt[1]
		}
	}
	return out
}

// Logger receives structured custody events.
type Logger interface {
	Write(event LogEvent) error
}

// Statuses written at warn level; everything else is info.
var warnStatuses = map[string]struct{}{
	"error": {}, "failed": {}, "timed_out": {}, "retry_exhausted": {}, "delivery_failed": {},
	StatusRetry: {}, StatusRejected: {},
}

// Default logger writing one "custody" line per event through smplog.
type smplogLogger struct{}

func (smplogLogger) Write(e LogEvent) error {
	if _, warn := warnStatuses[e.Status]; warn {
		logs.Warnf("custody %s", e.Line())
		return nil
	}
	logs.Infof("custody %s", e.Line())
	return nil
}

var (
	loggerMu sync.RWMutex
	logger   Logger = smplogLogger{}
)

// SetLogger replaces the custody logger and returns a restore func; intended for tests.
func SetLogger(l Logger) (restore func()) {
	loggerMu.Lock()
	prev := logger
	logger = l
	loggerMu.Unlock()
	return func() {
		loggerMu.Lock()
		logger = prev
		loggerMu.Unlock()
	}
}

// Emit stamps timestamp_ms when unset, derives trace_id/request_id when unset, and writes the
// event. Events still missing required fields are written with an error naming them, so gaps
// are visible rather than silently dropped.
func Emit(e LogEvent) {
	if e.TimestampMS == 0 {
		e.TimestampMS = uint64(time.Now().UnixMilli())
	}
	if e.TraceID == "" {
		e.TraceID = TraceID(e.IntentID)
	}
	if e.RequestID == "" {
		e.RequestID = RequestID(e.MessageType, e.IntentID, e.CommandID, e.EventID)
	}
	if err := e.Validate(); err != nil && e.Error == "" {
		e.Error = err.Error()
	}
	// Kept in memory as well so api/v1 can serve recent activity (api_v1.toml [parity.activity]).
	activity.Add(activity.KindCustody, e.TimestampMS, e.fields())
	loggerMu.RLock()
	l := logger
	loggerMu.RUnlock()
	// A custody line that cannot be written is reported, not dropped silently.
	if err := l.Write(e); err != nil {
		logs.Errf("custody write failed err=%v line=%q", err, e.Line())
	}
}

// TraceID derives the per-intent trace shared by every custody step on both runtimes.
// Empty when intent_id is unknown.
func TraceID(intentID string) string {
	id := strings.TrimSpace(intentID)
	if id == "" {
		return ""
	}
	return "trace." + id
}

// RequestID selects the correlation key linking a message to its custody predecessor
// (observability.toml [correlation_rules]): intent_id for issue/report, event_id for event.ack,
// command_id for command, seed.execute, seed.result, and event.
func RequestID(messageType uint32, intentID, commandID, eventID string) string {
	switch messageType {
	case schema.MsgIssue, schema.MsgReport:
		return strings.TrimSpace(intentID)
	case schema.MsgEventAck:
		return strings.TrimSpace(eventID)
	case schema.MsgCommand, schema.MsgSeedExecute, schema.MsgSeedResult, schema.MsgEvent:
		return strings.TrimSpace(commandID)
	}
	return ""
}
