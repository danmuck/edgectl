package main

// admin_mirage.go defines the MirageAdmin interface and RemoteMirageAdmin TCP client implementation.

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
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
	DeployGhost(req mirage.DeployGhostRequest) (mirage.DeployGhostResult, error)
	RegisteredGhosts() ([]mirage.RegisteredGhost, error)
	RoutingTable() ([]MirageRoute, error)
	AvailableServices() ([]MirageAvailableService, error)
	Close() error
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

func (c *RemoteMirageAdmin) DeployGhost(req mirage.DeployGhostRequest) (mirage.DeployGhostResult, error) {
	var out mirage.DeployGhostResult
	controlReq := mirageControlRequest{
		Action: "deploy_ghost",
		Deploy: req,
	}
	if err := c.call(controlReq, &out); err != nil {
		return mirage.DeployGhostResult{}, err
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
