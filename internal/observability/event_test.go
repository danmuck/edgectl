package observability

import (
	"errors"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/schema"
)

type recorder struct{ events []LogEvent }

func (r *recorder) Write(e LogEvent) error {
	r.events = append(r.events, e)
	return nil
}

func TestValidateNamesEveryMissingRequiredField(t *testing.T) {
	err := LogEvent{}.Validate()
	if !errors.Is(err, ErrMissingLogFields) {
		t.Fatalf("expected ErrMissingLogFields, got %v", err)
	}
	want := "component,peer,direction,trace_id,request_id,message_id,message_type,status,timestamp_ms"
	if !strings.HasSuffix(err.Error(), want) {
		t.Fatalf("missing list = %q, want suffix %q", err.Error(), want)
	}

	full := LogEvent{
		Component: "mirage", Peer: "ghost.alpha", Direction: DirectionOutbound, TraceID: "trace.i",
		RequestID: "cmd.i.1", MessageID: 7, MessageType: schema.MsgCommand, Status: StatusDispatched, TimestampMS: 1,
	}
	if err := full.Validate(); err != nil {
		t.Fatalf("complete event invalid: %v", err)
	}
}

func TestLineIsDeterministicRequiredFirst(t *testing.T) {
	e := LogEvent{
		Component: "ghost", Peer: "seed.flow", Direction: DirectionInbound, TraceID: "trace.i", RequestID: "cmd.i.1",
		MessageID: 9, MessageType: schema.MsgSeedResult, Status: "ok", TimestampMS: 42,
		IntentID: "i", CommandID: "cmd.i.1", ExecutionID: "exec.cmd.i.1",
	}
	want := `component="ghost" peer="seed.flow" direction="inbound" trace_id="trace.i" request_id="cmd.i.1" ` +
		`message_id=9 message_type=4 status="ok" timestamp_ms=42 intent_id="i" command_id="cmd.i.1" execution_id="exec.cmd.i.1"`
	if got := e.Line(); got != want {
		t.Fatalf("line mismatch\n got: %s\nwant: %s", got, want)
	}
}

func TestCorrelationDerivation(t *testing.T) {
	if TraceID("intent.a") != "trace.intent.a" || TraceID(" ") != "" {
		t.Fatalf("unexpected trace derivation")
	}
	cases := map[uint32]string{
		schema.MsgIssue:       "intent.a",
		schema.MsgReport:      "intent.a",
		schema.MsgCommand:     "cmd.a.1",
		schema.MsgSeedExecute: "cmd.a.1",
		schema.MsgSeedResult:  "cmd.a.1",
		schema.MsgEvent:       "cmd.a.1",
		schema.MsgEventAck:    "evt.cmd.a.1",
	}
	for mt, want := range cases {
		if got := RequestID(mt, "intent.a", "cmd.a.1", "evt.cmd.a.1"); got != want {
			t.Errorf("message_type=%d request_id=%q, want %q", mt, got, want)
		}
	}
}

func TestEmitDerivesIDsStampsTimeAndFlagsGaps(t *testing.T) {
	r := &recorder{}
	restore := SetLogger(r)
	defer restore()

	Emit(LogEvent{
		Component: "mirage", Peer: "ghost.alpha", Direction: DirectionOutbound,
		MessageID: 3, MessageType: schema.MsgCommand, IntentID: "intent.a", CommandID: "cmd.a.1", Status: StatusDispatched,
	})
	Emit(LogEvent{Component: "mirage", Status: "x"})

	if len(r.events) != 2 {
		t.Fatalf("recorded %d events, want 2", len(r.events))
	}
	ok := r.events[0]
	if ok.TraceID != "trace.intent.a" || ok.RequestID != "cmd.a.1" || ok.TimestampMS == 0 || ok.Error != "" {
		t.Fatalf("derived fields wrong: %+v", ok)
	}
	if !strings.Contains(r.events[1].Error, "missing required log fields") {
		t.Fatalf("incomplete event not flagged: %+v", r.events[1])
	}
}
