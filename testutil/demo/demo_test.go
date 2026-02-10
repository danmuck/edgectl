package demo

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
	logs "github.com/danmuck/smplog"
)

// demoExecutor provides deterministic command execution for control-flow demonstrations.
type demoExecutor struct {
	ghostID string
	count   int
}

// ExecuteCommand returns one success event for each received command.
func (e *demoExecutor) ExecuteCommand(_ context.Context, cmd session.Command) (session.Event, error) {
	e.count++
	return session.Event{
		EventID:     fmt.Sprintf("evt.%s", cmd.CommandID),
		CommandID:   cmd.CommandID,
		IntentID:    cmd.IntentID,
		GhostID:     e.ghostID,
		SeedID:      cmd.SeedSelector,
		Outcome:     mirage.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}, nil
}

// TestDemoProtocolMessageFlow demonstrates frame/TLV message flow round-trips.
func TestDemoProtocolMessageFlow(t *testing.T) {
	testlog.Start(t)
	logs.Debug("demo protocol flow: start")

	command := session.Command{
		CommandID:    "cmd.demo.1",
		IntentID:     "intent.demo.1",
		GhostID:      "ghost.alpha",
		SeedSelector: "seed.flow",
		Operation:    "status",
		Args:         map[string]string{"mode": "full"},
	}
	commandWire, err := session.EncodeCommandFrame(101, command)
	if err != nil {
		t.Fatalf("encode command: %v", err)
	}
	logs.Debug(fmt.Sprintf("demo protocol flow: encoded command bytes=%d", len(commandWire)))

	commandFrame, err := frame.ReadFrame(bytes.NewReader(commandWire), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read command frame: %v", err)
	}
	commandOut, err := session.DecodeCommandFrame(commandFrame)
	if err != nil {
		t.Fatalf("decode command frame: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo protocol flow: command decoded command_id=%s ghost_id=%s op=%s",
		commandOut.CommandID,
		commandOut.GhostID,
		commandOut.Operation,
	))

	event := session.Event{
		EventID:     "evt.demo.1",
		CommandID:   commandOut.CommandID,
		IntentID:    commandOut.IntentID,
		GhostID:     commandOut.GhostID,
		SeedID:      commandOut.SeedSelector,
		Outcome:     mirage.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	eventWire, err := session.EncodeEventFrame(102, event)
	if err != nil {
		t.Fatalf("encode event: %v", err)
	}
	eventFrame, err := frame.ReadFrame(bytes.NewReader(eventWire), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read event frame: %v", err)
	}
	eventOut, err := session.DecodeEventFrame(eventFrame)
	if err != nil {
		t.Fatalf("decode event frame: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo protocol flow: event decoded event_id=%s outcome=%s",
		eventOut.EventID,
		eventOut.Outcome,
	))

	ack := session.EventAck{
		EventID:     eventOut.EventID,
		CommandID:   eventOut.CommandID,
		GhostID:     eventOut.GhostID,
		AckStatus:   session.AckStatusAccepted,
		AckCode:     0,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ackWire, err := session.EncodeEventAckFrame(103, ack)
	if err != nil {
		t.Fatalf("encode event.ack: %v", err)
	}
	ackFrame, err := frame.ReadFrame(bytes.NewReader(ackWire), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read event.ack frame: %v", err)
	}
	ackOut, err := session.DecodeEventAckFrame(ackFrame)
	if err != nil {
		t.Fatalf("decode event.ack frame: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo protocol flow: ack decoded event_id=%s ack_status=%s",
		ackOut.EventID,
		ackOut.AckStatus,
	))

	report := session.Report{
		IntentID:        eventOut.IntentID,
		Phase:           mirage.ReportPhaseComplete,
		Summary:         "demo protocol flow complete",
		CompletionState: mirage.CompletionSatisfied,
		CommandID:       eventOut.CommandID,
		EventID:         eventOut.EventID,
		Outcome:         eventOut.Outcome,
		TimestampMS:     uint64(time.Now().UnixMilli()),
	}
	reportWire, err := session.EncodeReportFrame(104, report)
	if err != nil {
		t.Fatalf("encode report: %v", err)
	}
	reportFrame, err := frame.ReadFrame(bytes.NewReader(reportWire), frame.DefaultLimits())
	if err != nil {
		t.Fatalf("read report frame: %v", err)
	}
	reportOut, err := session.DecodeReportFrame(reportFrame)
	if err != nil {
		t.Fatalf("decode report frame: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo protocol flow: report decoded intent_id=%s phase=%s completion=%s",
		reportOut.IntentID,
		reportOut.Phase,
		reportOut.CompletionState,
	))

	if reportOut.IntentID != report.IntentID || reportOut.Phase != mirage.ReportPhaseComplete {
		t.Fatalf("unexpected report decode output: %+v", reportOut)
	}
}

// TestDemoOrchestratorControlFlow demonstrates a high-level reconcile flow in the Mirage orchestrator.
func TestDemoOrchestratorControlFlow(t *testing.T) {
	testlog.Start(t)
	logs.Debug("demo orchestrator flow: start")

	loop := mirage.NewOrchestrator()
	exec := &demoExecutor{ghostID: "ghost.alpha"}
	if err := loop.RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}

	issue := mirage.IssueEnv{
		IntentID:    "intent.demo.orch.1",
		Actor:       "user:demo",
		TargetScope: "ghost:ghost.alpha",
		Objective:   "demonstrate reconcile loop",
		Stages: []mirage.IssueStage{
			{
				ID:      "stage.1",
				Barrier: true,
				Commands: []mirage.IssueCommand{
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
					{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
				},
			},
		},
	}
	if err := loop.SubmitIssue(issue); err != nil {
		t.Fatalf("submit issue: %v", err)
	}
	logs.Debug(fmt.Sprintf("demo orchestrator flow: submitted intent_id=%s", issue.IntentID))

	reportA, err := loop.ReconcileOnce(context.Background(), issue.IntentID)
	if err != nil {
		t.Fatalf("reconcile A: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo orchestrator flow: cycle=1 phase=%s completion=%s command_id=%s",
		reportA.Phase,
		reportA.CompletionState,
		reportA.CommandID,
	))

	reportB, err := loop.ReconcileOnce(context.Background(), issue.IntentID)
	if err != nil {
		t.Fatalf("reconcile B: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo orchestrator flow: cycle=2 phase=%s completion=%s command_id=%s",
		reportB.Phase,
		reportB.CompletionState,
		reportB.CommandID,
	))

	if reportA.Phase != mirage.ReportPhaseInProgress {
		t.Fatalf("expected in_progress after first reconcile, got=%s", reportA.Phase)
	}
	if reportB.Phase != mirage.ReportPhaseComplete {
		t.Fatalf("expected complete after second reconcile, got=%s", reportB.Phase)
	}
	if reportB.CompletionState != mirage.CompletionSatisfied {
		t.Fatalf("expected satisfied completion, got=%s", reportB.CompletionState)
	}
}

// TestDemoMirageGhostE2EProtocol demonstrates handshake + event + event.ack over a live session.
func TestDemoMirageGhostE2EProtocol(t *testing.T) {
	testlog.Start(t)
	logs.Debug("demo e2e protocol: start listener and mirage service")

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	cfg := mirage.DefaultServiceConfig()
	cfg.RequireIdentityBinding = true
	cfg.Session.ConnectTimeout = 2 * time.Second
	cfg.Session.HandshakeTimeout = 2 * time.Second
	cfg.Session.ReadTimeout = 2 * time.Second
	cfg.Session.WriteTimeout = 2 * time.Second
	svc := mirage.NewServiceWithConfig(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- svc.Serve(ctx, ln)
	}()

	client, err := ghost.NewMirageClient(ghost.MirageClientConfig{
		Address:      ln.Addr().String(),
		GhostID:      "ghost.demo",
		PeerIdentity: "ghost.demo",
		SeedList: []session.SeedInfo{
			{ID: "seed.flow", Name: "Flow", Description: "Deterministic control-flow seed"},
		},
		Session: session.Config{
			ConnectTimeout:   2 * time.Second,
			HandshakeTimeout: 2 * time.Second,
			ReadTimeout:      2 * time.Second,
			WriteTimeout:     2 * time.Second,
			AckTimeout:       3 * time.Second,
			Backoff:          session.DefaultConfig().Backoff,
		},
		MaxConnectAttempts: 1,
	})
	if err != nil {
		cancel()
		_ = <-done
		t.Fatalf("new mirage client: %v", err)
	}

	connectCtx, connectCancel := context.WithTimeout(ctx, 3*time.Second)
	defer connectCancel()
	gs, err := client.ConnectAndRegister(connectCtx)
	if err != nil {
		cancel()
		_ = <-done
		t.Fatalf("connect and register: %v", err)
	}
	defer gs.Close()
	logs.Debug("demo e2e protocol: registration accepted, session ready")

	event := ghost.EventEnv{
		EventID:     "evt.demo.e2e.1",
		CommandID:   "cmd.demo.e2e.1",
		IntentID:    "intent.demo.e2e.1",
		GhostID:     "ghost.demo",
		SeedID:      "seed.flow",
		Outcome:     ghost.OutcomeSuccess,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	ack, err := gs.SendEventWithAck(connectCtx, event)
	if err != nil {
		cancel()
		_ = <-done
		t.Fatalf("send event with ack: %v", err)
	}
	logs.Debug(fmt.Sprintf(
		"demo e2e protocol: event delivered event_id=%s ack_status=%s ack_code=%d",
		ack.EventID,
		ack.AckStatus,
		ack.AckCode,
	))

	if ack.AckStatus != session.AckStatusAccepted {
		t.Fatalf("expected accepted ack, got=%s", ack.AckStatus)
	}
	if ack.EventID != event.EventID {
		t.Fatalf("ack event_id mismatch: got=%s want=%s", ack.EventID, event.EventID)
	}

	cancel()
	if err := <-done; err != nil {
		t.Fatalf("service exit error: %v", err)
	}
}
