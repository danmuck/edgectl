package mirage

import (
	"bufio"
	"encoding/json"
	"net"
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

	cfg := DefaultServiceConfig()
	cfg.LocalGhostID = ""
	cfg.LocalGhostAdminAddr = ""
	svc := NewServiceWithConfig(cfg)
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

func TestHandleAdminControlRegisteredGhostsIncludesLocalGhost(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 3; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			reader := bufio.NewReader(conn)
			line, err := reader.ReadBytes('\n')
			if err != nil {
				_ = conn.Close()
				return
			}
			var req ghostControlRequest
			if err := json.Unmarshal(line, &req); err != nil {
				_ = conn.Close()
				return
			}
			resp := ghostControlResponse{OK: true}
			switch req.Action {
			case bindMirageAction:
				// no payload required for this test.
			case statusAction:
				resp.Data = mustJSON(t, map[string]any{
					"GhostID": "ghost.local",
				})
			case listSeedsAction:
				resp.Data = mustJSON(t, []map[string]any{
					{"id": "seed.flow", "name": "Flow"},
				})
			default:
				_ = conn.Close()
				return
			}
			payload, _ := json.Marshal(resp)
			payload = append(payload, '\n')
			_, _ = conn.Write(payload)
			_ = conn.Close()
		}
	}()

	cfg := DefaultServiceConfig()
	cfg.LocalGhostID = "ghost.local"
	cfg.LocalGhostAdminAddr = ln.Addr().String()
	svc := NewServiceWithConfig(cfg)
	resp := svc.handleAdminControlRequest(adminControlRequest{Action: "registered_ghosts"})
	if !resp.OK {
		t.Fatalf("registered_ghosts failed: %+v", resp)
	}
	list, ok := resp.Data.([]RegisteredGhost)
	if !ok {
		t.Fatalf("unexpected data type: %T", resp.Data)
	}
	if len(list) == 0 {
		t.Fatalf("expected local ghost in list")
	}
	found := false
	for i := range list {
		if list[i].GhostID == "ghost.local" && list[i].Connected {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected connected local ghost entry, got %+v", list)
	}
	<-done
}

func TestHandleAdminControlAttachGhostAdmin(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 8; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			reader := bufio.NewReader(conn)
			line, err := reader.ReadBytes('\n')
			if err != nil {
				_ = conn.Close()
				return
			}
			var req ghostControlRequest
			if err := json.Unmarshal(line, &req); err != nil {
				_ = conn.Close()
				return
			}
			resp := ghostControlResponse{OK: true}
			switch req.Action {
			case bindMirageAction:
				// no payload required for this test.
			case statusAction:
				resp.Data = mustJSON(t, map[string]any{
					"GhostID": "ghost.remote.a",
				})
			case listSeedsAction:
				resp.Data = mustJSON(t, []map[string]any{
					{"id": "seed.flow", "name": "Flow"},
				})
			default:
				_ = conn.Close()
				return
			}
			payload, _ := json.Marshal(resp)
			payload = append(payload, '\n')
			_, _ = conn.Write(payload)
			_ = conn.Close()
		}
	}()

	svc := NewServiceWithConfig(DefaultServiceConfig())
	resp := svc.handleAdminControlRequest(adminControlRequest{
		Action:         "attach_ghost_admin",
		GhostAdminAddr: ln.Addr().String(),
	})
	if !resp.OK {
		t.Fatalf("attach_ghost_admin failed: %+v", resp)
	}
	out, ok := resp.Data.(AdminAttachGhostResponse)
	if !ok {
		t.Fatalf("unexpected data type: %T", resp.Data)
	}
	if out.GhostID != "ghost.remote.a" {
		t.Fatalf("unexpected attach response: %+v", out)
	}
	reports := svc.handleAdminControlRequest(adminControlRequest{Action: "registered_ghosts"})
	if !reports.OK {
		t.Fatalf("registered_ghosts after attach failed: %+v", reports)
	}
	list, ok := reports.Data.([]RegisteredGhost)
	if !ok {
		t.Fatalf("unexpected list type: %T", reports.Data)
	}
	found := false
	for i := range list {
		if list[i].GhostID == "ghost.remote.a" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("attached ghost missing from list: %+v", list)
	}
	routes := svc.handleAdminControlRequest(adminControlRequest{Action: "routing_table"})
	if !routes.OK {
		t.Fatalf("routing_table failed: %+v", routes)
	}
	routeList, ok := routes.Data.([]GhostRoute)
	if !ok {
		t.Fatalf("unexpected routing_table type: %T", routes.Data)
	}
	if len(routeList) == 0 || routeList[0].GhostID == "" {
		t.Fatalf("unexpected routing_table payload: %+v", routeList)
	}
	services := svc.handleAdminControlRequest(adminControlRequest{Action: "available_services"})
	if !services.OK {
		t.Fatalf("available_services failed: %+v", services)
	}
	serviceList, ok := services.Data.([]AvailableService)
	if !ok {
		t.Fatalf("unexpected available_services type: %T", services.Data)
	}
	if len(serviceList) == 0 || serviceList[0].SeedID == "" {
		t.Fatalf("unexpected available_services payload: %+v", serviceList)
	}
	<-done
}
