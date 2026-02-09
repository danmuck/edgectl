package main

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

// MirageAdmin defines the client control boundary for one Mirage target.
type MirageAdmin interface {
	Address() string
	Status() (mirage.LifecycleStatus, error)
	SubmitIssue(issue MirageIssueRequest) error
	ReconcileIntent(intentID string) (session.Report, error)
	ReconcileAll() ([]session.Report, error)
	SnapshotIntent(intentID string) (mirage.IntentSnapshot, bool, error)
	ListIntents() ([]string, error)
	RecentReports(limit int) ([]session.Report, error)
	SpawnLocalGhost(req mirage.SpawnGhostRequest) (mirage.SpawnGhostResult, error)
	AttachGhostAdmin(addr string) (MirageAttachGhostResponse, error)
	RegisteredGhosts() ([]mirage.RegisteredGhost, error)
	RoutingTable() ([]MirageRoute, error)
	AvailableServices() ([]MirageAvailableService, error)
	Close() error
}

func (a *App) printMirageMenu() {
	fmt.Println()
	fmt.Println("Client TM (Mirage)")
	fmt.Printf("  ghost config:  %s (targets=%d)\n", a.ghostCfgPath, len(a.ghostCfg.Targets))
	fmt.Printf("  mirage config: %s (single control plane)\n", a.mirageCfgPath)
	fmt.Printf("  clear screen after command: %v\n", a.clearScreen)
	fmt.Println("  (*) not yet fully implemented")
	fmt.Println("  1) Show mirage control-plane config")
	fmt.Println("  2) Show mirage status")
	fmt.Println("  3) Mirage admin console (*)")
	fmt.Println("  4) Show connected ghosts")
	fmt.Println("  5) Open local ghost admin console")
	fmt.Println("  6) Config menu")
	fmt.Println("  7) Exit")
}

func (a *App) runMirageAdminConsole() error {
	target, ok := a.activeMirageTarget()
	if !ok {
		return errors.New("no active mirage target")
	}
	for {
		fmt.Println()
		fmt.Printf("Mirage Admin Console (%s @ %s)\n", target.Name, target.Admin.Address())
		fmt.Println("  (*) not yet fully implemented")
		fmt.Println("  1) Show status")
		fmt.Println("  2) Show available services")
		fmt.Println("  3) Issue intent")
		fmt.Println("  4) Reconcile intent")
		fmt.Println("  5) Reports")
		fmt.Println("  6) Ghost routing")
		fmt.Println("  7) Back")
		choice, err := a.promptInt("Choose", 1, 7, true, true)
		if err != nil {
			if errors.Is(err, ErrNavigateBack) {
				return nil
			}
			return err
		}
		a.clearIfEnabled()
		switch choice {
		case 1:
			if err := a.showActiveMirageSummary(); err != nil {
				logs.Errf("show mirage summary failed: %v", err)
			}
		case 2:
			if err := a.showMirageAvailableServices(target); err != nil {
				logs.Errf("show available services failed: %v", err)
			}
		case 3:
			if err := a.submitMirageIssue(target); err != nil {
				logs.Errf("issue intent failed: %v", err)
			}
		case 4:
			if err := a.runMirageReconcileConsole(target); err != nil {
				logs.Errf("reconcile console failed: %v", err)
			}
		case 5:
			if err := a.runMirageReportsConsole(target); err != nil {
				logs.Errf("reports console failed: %v", err)
			}
		case 6:
			if err := a.runMirageGhostRoutingConsole(target); err != nil {
				logs.Errf("ghost routing console failed: %v", err)
			}
		case 7:
			return nil
		}
	}
}
func NewRemoteMirageAdmin(addr string) *RemoteMirageAdmin {
	return &RemoteMirageAdmin{addr: strings.TrimSpace(addr)}
}

func (c *RemoteMirageAdmin) Address() string {
	return c.addr
}

func (c *RemoteMirageAdmin) Status() (mirage.LifecycleStatus, error) {
	var out mirage.LifecycleStatus
	if err := c.call(mirageControlRequest{Action: "status"}, &out); err != nil {
		return mirage.LifecycleStatus{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) SubmitIssue(issue MirageIssueRequest) error {
	return c.call(mirageControlRequest{Action: "submit_issue", Issue: issue}, nil)
}

func (c *RemoteMirageAdmin) ReconcileIntent(intentID string) (session.Report, error) {
	var out session.Report
	req := mirageControlRequest{
		Action:   "reconcile_intent",
		IntentID: strings.TrimSpace(intentID),
	}
	if err := c.call(req, &out); err != nil {
		return session.Report{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) ReconcileAll() ([]session.Report, error) {
	var out mirageReconcileAllResponse
	if err := c.call(mirageControlRequest{Action: "reconcile_all"}, &out); err != nil {
		return nil, err
	}
	return out.Reports, nil
}

func (c *RemoteMirageAdmin) SnapshotIntent(intentID string) (mirage.IntentSnapshot, bool, error) {
	var out mirageSnapshotIntentResponse
	req := mirageControlRequest{
		Action:   "snapshot_intent",
		IntentID: strings.TrimSpace(intentID),
	}
	if err := c.call(req, &out); err != nil {
		return mirage.IntentSnapshot{}, false, err
	}
	return out.Snapshot, out.Found, nil
}

func (c *RemoteMirageAdmin) ListIntents() ([]string, error) {
	var out []string
	if err := c.call(mirageControlRequest{Action: "list_intents"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) RecentReports(limit int) ([]session.Report, error) {
	var out []session.Report
	if err := c.call(mirageControlRequest{Action: "recent_reports", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) SpawnLocalGhost(req mirage.SpawnGhostRequest) (mirage.SpawnGhostResult, error) {
	var out mirage.SpawnGhostResult
	controlReq := mirageControlRequest{
		Action: "spawn_local_ghost",
		Spawn:  req,
	}
	if err := c.call(controlReq, &out); err != nil {
		return mirage.SpawnGhostResult{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) AttachGhostAdmin(addr string) (MirageAttachGhostResponse, error) {
	var out MirageAttachGhostResponse
	req := mirageControlRequest{
		Action:         "attach_ghost_admin",
		GhostAdminAddr: strings.TrimSpace(addr),
	}
	if err := c.call(req, &out); err != nil {
		return MirageAttachGhostResponse{}, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) RegisteredGhosts() ([]mirage.RegisteredGhost, error) {
	var out []mirage.RegisteredGhost
	if err := c.call(mirageControlRequest{Action: "registered_ghosts"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) RoutingTable() ([]MirageRoute, error) {
	var out []MirageRoute
	if err := c.call(mirageControlRequest{Action: "routing_table"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) AvailableServices() ([]MirageAvailableService, error) {
	var out []MirageAvailableService
	if err := c.call(mirageControlRequest{Action: "available_services"}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteMirageAdmin) call(req mirageControlRequest, out any) error {
	if err := c.ensureConn(); err != nil {
		return err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.conn.Write(payload); err != nil {
		c.resetConn()
		return err
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		c.resetConn()
		return err
	}
	var resp mirageControlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

func (c *RemoteMirageAdmin) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *RemoteMirageAdmin) resetConn() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.r = nil
}

func (c *RemoteMirageAdmin) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}
