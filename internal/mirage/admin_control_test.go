package mirage

import (
	"testing"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestHandleAdminControlSubmitAndReconcileIntent(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	exec := &fakeExecutor{}
	if err := svc.Server().RegisterExecutor("ghost.alpha", exec); err != nil {
		t.Fatalf("register executor: %v", err)
	}

	submitResp := svc.handleAdminControlRequest(adminControlRequest{
		Action: "submit_issue",
		Issue: AdminIssueRequest{
			IntentID:    "intent.admin.1",
			Actor:       "user:dan",
			TargetScope: "ghost:ghost.alpha",
			Objective:   "status",
			CommandPlan: []AdminIssueCommand{
				{GhostID: "ghost.alpha", SeedSelector: "seed.flow", Operation: "status"},
			},
		},
	})
	if !submitResp.OK {
		t.Fatalf("submit failed: %+v", submitResp)
	}

	reconcileResp := svc.handleAdminControlRequest(adminControlRequest{
		Action:   "reconcile_intent",
		IntentID: "intent.admin.1",
	})
	if !reconcileResp.OK {
		t.Fatalf("reconcile failed: %+v", reconcileResp)
	}

	reportsResp := svc.handleAdminControlRequest(adminControlRequest{
		Action: "recent_reports",
		Limit:  10,
	})
	if !reportsResp.OK {
		t.Fatalf("recent_reports failed: %+v", reportsResp)
	}
}

func TestHandleAdminControlRegisteredGhosts(t *testing.T) {
	testlog.Start(t)

	svc := NewServiceWithConfig(DefaultServiceConfig())
	svc.Server().UpsertRegistration("127.0.0.1:41000", session.Registration{
		GhostID:      "ghost.alpha",
		PeerIdentity: "ghost.alpha",
	})
	resp := svc.handleAdminControlRequest(adminControlRequest{
		Action: "registered_ghosts",
	})
	if !resp.OK {
		t.Fatalf("registered_ghosts failed: %+v", resp)
	}
	list, ok := resp.Data.([]RegisteredGhost)
	if !ok {
		t.Fatalf("unexpected data type: %T", resp.Data)
	}
	if len(list) != 1 || list[0].GhostID != "ghost.alpha" {
		t.Fatalf("unexpected list payload: %+v", list)
	}
}
