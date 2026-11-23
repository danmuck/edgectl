package ghost

import (
	"testing"

	"github.com/danmuck/edgectl/internal/observability"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/testutil/custodytest"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func commandStatuses(events []observability.LogEvent) []string {
	var out []string
	for _, e := range events {
		if e.MessageType == schema.MsgCommand {
			out = append(out, e.Status)
		}
	}
	return out
}

// Replay (idempotency) is logged as replayed with no second seed.execute/seed.result.
func TestCustodyLinesForCommandReplay(t *testing.T) {
	testlog.Start(t)
	capture := custodytest.Install(t)
	s, _ := newCountingServer(t)

	first := bumpCommand(1)
	first.Peer = "mirage.test"
	if _, err := s.HandleCommandAndExecute(first); err != nil {
		t.Fatalf("first: %v", err)
	}
	retry := bumpCommand(2)
	retry.Peer = "mirage.test"
	if _, err := s.HandleCommandAndExecute(retry); err != nil {
		t.Fatalf("replay: %v", err)
	}

	events := capture.ForTrace(observability.TraceID("intent.idem.1"))
	custodytest.RequireValid(t, events)
	custodytest.RequireOrderedSteps(t, events, []custodytest.Step{
		custodytest.S("ghost", schema.MsgCommand, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgSeedExecute, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgSeedResult, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgEvent, observability.DirectionOutbound),
		custodytest.S("ghost", schema.MsgCommand, observability.DirectionInbound),
		custodytest.S("ghost", schema.MsgEvent, observability.DirectionOutbound),
	})
	if got := commandStatuses(events); len(got) != 2 || got[0] != observability.StatusAccepted || got[1] != observability.StatusReplayed {
		t.Fatalf("command statuses = %v, want [accepted replayed]", got)
	}
	seedLines := 0
	for _, e := range events {
		if e.MessageType == schema.MsgSeedExecute {
			seedLines++
		}
		if e.Peer == "unknown" {
			t.Errorf("peer not propagated: %s", e.Line())
		}
	}
	if seedLines != 1 {
		t.Fatalf("seed.execute lines = %d, want 1 (replay must not re-execute)", seedLines)
	}
}

// A rejected command still produces a complete custody line carrying the rejection reason.
func TestCustodyLineForRejectedCommand(t *testing.T) {
	testlog.Start(t)
	capture := custodytest.Install(t)
	s, _ := newCountingServer(t)

	cmd := bumpCommand(5)
	cmd.GhostID = "ghost.other"
	cmd.Peer = "mirage.test"
	if _, err := s.HandleCommandAndExecute(cmd); err == nil {
		t.Fatalf("expected target mismatch")
	}
	events := capture.ForTrace(observability.TraceID("intent.idem.1"))
	custodytest.RequireValid(t, events)
	if len(events) != 1 || events[0].Status != observability.StatusRejected || events[0].Error == "" {
		t.Fatalf("expected one rejected line with error, got %+v", events)
	}
}
