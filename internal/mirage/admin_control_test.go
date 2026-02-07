package mirage

import (
	"testing"

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
