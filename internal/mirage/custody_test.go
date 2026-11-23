package mirage

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
	seedflow "github.com/danmuck/edgectl/internal/seeds/flow"
	"github.com/danmuck/edgectl/internal/testutil/custodytest"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

// Executor that hands Mirage commands to an in-process Ghost server.
type inProcessGhost struct {
	srv *ghost.Server
	seq atomic.Uint64
}

func (g *inProcessGhost) ExecuteCommand(_ context.Context, cmd session.Command) (session.Event, error) {
	ev, err := g.srv.HandleCommandAndExecute(ghost.CommandEnv{
		MessageID:    g.seq.Add(1),
		CommandID:    cmd.CommandID,
		IntentID:     cmd.IntentID,
		GhostID:      cmd.GhostID,
		SeedSelector: cmd.SeedSelector,
		Operation:    cmd.Operation,
		Args:         cmd.Args,
		Peer:         "mirage.local",
	})
	if err != nil {
		return session.Event{}, err
	}
	return session.Event{
		EventID: ev.EventID, CommandID: ev.CommandID, IntentID: ev.IntentID, GhostID: ev.GhostID,
		SeedID: ev.SeedID, Outcome: ev.Outcome, TimestampMS: ev.TimestampMS,
	}, nil
}

func newRadiatingGhost(t *testing.T) *ghost.Server {
	t.Helper()
	srv := ghost.NewServer()
	if err := srv.Appear(ghost.GhostConfig{GhostID: "ghost.alpha"}); err != nil {
		t.Fatalf("appear: %v", err)
	}
	reg := seeds.NewRegistry()
	if err := reg.Register(seedflow.NewSeed()); err != nil {
		t.Fatalf("register flow: %v", err)
	}
	if err := srv.Seed(reg); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := srv.Radiate(); err != nil {
		t.Fatalf("radiate: %v", err)
	}
	return srv
}

func submitStatusIssue(t *testing.T, srv *Server, intentID string) {
	t.Helper()
	if err := srv.SubmitIssue(IssueEnv{
		IntentID:    intentID,
		Actor:       "user:test",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "status",
		Stages: []IssueStage{{
			ID:       "stage.1",
			Commands: []IssueCommand{{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"}},
		}},
	}); err != nil {
		t.Fatalf("submit issue: %v", err)
	}
}

// Success flow: issue -> command -> seed.execute -> seed.result -> event -> report share one trace,
// every line carries all required fields, and request_id follows the correlation rules.
func TestCustodyChainSuccessAcrossMirageAndGhost(t *testing.T) {
	testlog.Start(t)
	capture := custodytest.Install(t)

	const intentID = "intent.obs.success"
	srv := NewServer()
	if err := srv.RegisterExecutor("ghost.alpha", &inProcessGhost{srv: newRadiatingGhost(t)}); err != nil {
		t.Fatalf("register executor: %v", err)
	}
	submitStatusIssue(t, srv, intentID)
	if _, err := srv.ReconcileIntent(context.Background(), intentID); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	events := capture.ForTrace(observability.TraceID(intentID))
	custodytest.RequireValid(t, events)
	custodytest.RequireOrderedSteps(t, events, []custodytest.Step{
		custodytest.S("mirage", schema.MsgIssue, observability.DirectionInbound),
		custodytest.S("mirage", schema.MsgCommand, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgCommand, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgSeedExecute, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgSeedResult, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgEvent, observability.DirectionOutbound),
		custodytest.S("mirage", schema.MsgEvent, observability.DirectionInbound),
		custodytest.S("mirage", schema.MsgReport, observability.DirectionOutbound),
	})

	const commandID = "cmd." + intentID + ".1"
	for _, e := range events {
		switch e.MessageType {
		case schema.MsgIssue, schema.MsgReport:
			if e.RequestID != intentID {
				t.Errorf("%d request_id=%q, want intent_id", e.MessageType, e.RequestID)
			}
		default:
			if e.RequestID != commandID {
				t.Errorf("%d request_id=%q, want command_id %q", e.MessageType, e.RequestID, commandID)
			}
		}
		if (e.MessageType == schema.MsgSeedExecute || e.MessageType == schema.MsgSeedResult) && e.ExecutionID != "exec."+commandID {
			t.Errorf("seed line missing execution_id link: %s", e.Line())
		}
	}
}

// Failure flow: dispatch, every retry, the retry_exhausted terminal state, and the failed report
// are complete lines on the intent's trace.
func TestCustodyChainRetryExhaustedFailureFlow(t *testing.T) {
	testlog.Start(t)
	capture := custodytest.Install(t)

	exec := &scriptedExecutor{failures: []error{errTransport, errTransport, errTransport}}
	o := newRetryOrchestrator(t, exec, &recordingSleeper{})
	if _, err := o.ReconcileOnce(context.Background(), retryIntent); err != nil {
		t.Fatalf("reconcile: %v", err)
	}

	events := capture.ForTrace(observability.TraceID(retryIntent))
	custodytest.RequireValid(t, events)
	var statuses []string
	for _, e := range events {
		if e.Component == "mirage" && e.MessageType == schema.MsgCommand {
			statuses = append(statuses, e.Status)
		}
	}
	want := []string{observability.StatusDispatched, "retry", "retry", "retry", string(CommandRetryExhausted)}
	if len(statuses) != len(want) {
		t.Fatalf("command statuses = %v, want %v", statuses, want)
	}
	for i := range want {
		if statuses[i] != want[i] {
			t.Fatalf("command statuses = %v, want %v", statuses, want)
		}
	}
	last := events[len(events)-1]
	if last.MessageType != schema.MsgReport || last.Status != CompletionFailed {
		t.Fatalf("final line = %s, want failed report", last.Line())
	}
}

// Session flow: event -> event.ack -> report on both runtimes, request_id = event_id for acks,
// and a redelivered event is logged as duplicate with its ack.
func TestCustodyChainSessionEventAckAndDuplicate(t *testing.T) {
	testlog.Start(t)
	capture := custodytest.Install(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg := DefaultServiceConfig()
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second
	cfg.Session.HandshakeTimeout = 2 * time.Second
	svc := NewServiceWithConfig(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- svc.Serve(ctx, ln) }()

	const intentID = "intent.obs.session"
	submitStatusIssue(t, svc.server, intentID)

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address: ln.Addr().String(), GhostID: "ghost.alpha", PeerIdentity: "ghost.alpha",
		SeedList: []session.SeedInfo{{ID: "seed.flow", Name: "Flow", Description: "flow"}},
		Session: session.Config{
			ConnectTimeout: 2 * time.Second, HandshakeTimeout: 2 * time.Second, ReadTimeout: 2 * time.Second,
			WriteTimeout: 2 * time.Second, AckTimeout: 3 * time.Second, Backoff: session.DefaultConfig().Backoff,
		},
	})
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	opCtx, opCancel := context.WithTimeout(ctx, 5*time.Second)
	defer opCancel()
	gs, err := client.ConnectAndRegister(opCtx)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer gs.Close()

	const commandID = "cmd." + intentID + ".1"
	ev := ghost.EventEnv{
		EventID: "evt." + commandID, CommandID: commandID, IntentID: intentID, GhostID: "ghost.alpha",
		SeedID: "seed.flow", Outcome: ghost.OutcomeSuccess, TimestampMS: uint64(time.Now().UnixMilli()),
	}
	for i := 0; i < 2; i++ {
		if _, err := gs.SendEventWithAck(opCtx, ev); err != nil {
			t.Fatalf("send %d: %v", i+1, err)
		}
	}

	events := capture.ForTrace(observability.TraceID(intentID))
	custodytest.RequireValid(t, events)

	var mirageLines, ghostLines []observability.LogEvent
	for _, e := range events {
		switch e.Component {
		case "mirage":
			mirageLines = append(mirageLines, e)
		case "ghost":
			ghostLines = append(ghostLines, e)
		}
		if e.MessageType == schema.MsgEventAck && e.RequestID != ev.EventID {
			t.Errorf("event.ack request_id=%q, want event_id %q", e.RequestID, ev.EventID)
		}
	}
	custodytest.RequireOrderedSteps(t, mirageLines, []custodytest.Step{
		custodytest.S("mirage", schema.MsgIssue, observability.DirectionInbound),
		custodytest.S("mirage", schema.MsgEvent, observability.DirectionInbound),
		custodytest.S("mirage", schema.MsgReport, observability.DirectionOutbound),
		custodytest.S("mirage", schema.MsgEventAck, observability.DirectionOutbound),
		custodytest.S("mirage", schema.MsgEvent, observability.DirectionInbound),
		custodytest.S("mirage", schema.MsgEventAck, observability.DirectionOutbound),
	})
	custodytest.RequireOrderedSteps(t, ghostLines, []custodytest.Step{
		custodytest.S("ghost", schema.MsgEvent, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgEventAck, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgEvent, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgEventAck, observability.DirectionInbound),
	})

	var inbound []string
	for _, e := range mirageLines {
		if e.MessageType == schema.MsgEvent {
			inbound = append(inbound, e.Status)
		}
	}
	if len(inbound) != 2 || inbound[0] != OutcomeSuccess || inbound[1] != observability.StatusDuplicate {
		t.Fatalf("mirage inbound event statuses = %v, want [success duplicate]", inbound)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("serve exit err: %v", err)
	}
}
