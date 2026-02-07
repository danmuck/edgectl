package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	logs "github.com/danmuck/smplog"
)

// AdminIssueCommand defines one command step for mirage issue submission.
type AdminIssueCommand struct {
	GhostID      string            `json:"ghost_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
	Blocking     bool              `json:"blocking"`
}

// AdminIssueRequest defines one issue ingest request for mirage admin controls.
type AdminIssueRequest struct {
	IntentID    string              `json:"intent_id"`
	Actor       string              `json:"actor"`
	TargetScope string              `json:"target_scope"`
	Objective   string              `json:"objective"`
	CommandPlan []AdminIssueCommand `json:"command_plan"`
}

// AdminSnapshotIntentResponse captures one intent snapshot response payload.
type AdminSnapshotIntentResponse struct {
	Found    bool           `json:"found"`
	Snapshot IntentSnapshot `json:"snapshot"`
}

// AdminReconcileAllResponse captures bounded reconcile output for all known intents.
type AdminReconcileAllResponse struct {
	Reports []session.Report `json:"reports"`
}

type adminControlRequest struct {
	Action   string            `json:"action"`
	Limit    int               `json:"limit,omitempty"`
	IntentID string            `json:"intent_id,omitempty"`
	Issue    AdminIssueRequest `json:"issue,omitempty"`
	Spawn    SpawnGhostRequest `json:"spawn,omitempty"`
}

type adminControlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// serveAdminControl exposes a TCP JSON request/response endpoint for Mirage control.
func (s *Service) serveAdminControl(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	defer ln.Close()
	logs.Infof("mirage.admin listening addr=%q", ln.Addr().String())

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleAdminConn(conn)
	}
}

// handleAdminConn decodes one request per line and writes one response per line.
func (s *Service) handleAdminConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	active := s.adminClientCount.Add(1)
	logs.Infof("mirage.admin client connected remote=%q active_clients=%d", remote, active)
	defer func() {
		remaining := s.adminClientCount.Add(-1)
		logs.Infof("mirage.admin client disconnected remote=%q active_clients=%d", remote, remaining)
	}()
	reader := bufio.NewReader(conn)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logs.Warnf("mirage.admin read err=%v", err)
			}
			return
		}
		var req adminControlRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeAdminControlResponse(conn, adminControlResponse{OK: false, Error: err.Error()})
			continue
		}
		resp := s.handleAdminControlRequest(req)
		if err := writeAdminControlResponse(conn, resp); err != nil {
			logs.Warnf("mirage.admin write err=%v", err)
			return
		}
	}
}

// handleAdminControlRequest routes one admin action to Mirage runtime methods.
func (s *Service) handleAdminControlRequest(req adminControlRequest) adminControlResponse {
	switch strings.TrimSpace(req.Action) {
	case "status":
		return adminControlResponse{OK: true, Data: s.server.Status()}
	case "submit_issue":
		issue := mapAdminIssue(req.Issue)
		if err := s.server.SubmitIssue(issue); err != nil {
			return adminControlResponse{OK: false, Error: err.Error()}
		}
		s.persistBuildlog("submit_issue", map[string]any{
			"intent_id": issue.IntentID,
			"actor":     issue.Actor,
		})
		return adminControlResponse{OK: true}
	case "reconcile_intent":
		intentID := strings.TrimSpace(req.IntentID)
		if intentID == "" {
			return adminControlResponse{OK: false, Error: "intent_id required"}
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := s.server.ReconcileIntent(ctx, intentID)
		if err != nil {
			return adminControlResponse{OK: false, Error: err.Error()}
		}
		s.persistBuildlog("reconcile_intent", report)
		return adminControlResponse{OK: true, Data: report}
	case "reconcile_all":
		intentIDs := s.server.ListIntentIDs()
		reports := make([]session.Report, 0, len(intentIDs))
		for _, intentID := range intentIDs {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			report, err := s.server.ReconcileIntent(ctx, intentID)
			cancel()
			if err != nil && !errors.Is(err, ErrIntentNotFound) {
				return adminControlResponse{OK: false, Error: fmt.Sprintf("intent_id=%s: %v", intentID, err)}
			}
			if err == nil {
				reports = append(reports, report)
			}
		}
		s.persistBuildlog("reconcile_all", map[string]any{"count": len(reports)})
		return adminControlResponse{OK: true, Data: AdminReconcileAllResponse{Reports: reports}}
	case "snapshot_intent":
		intentID := strings.TrimSpace(req.IntentID)
		if intentID == "" {
			return adminControlResponse{OK: false, Error: "intent_id required"}
		}
		snapshot, found := s.server.SnapshotIntent(intentID)
		return adminControlResponse{OK: true, Data: AdminSnapshotIntentResponse{
			Found:    found,
			Snapshot: snapshot,
		}}
	case "list_intents":
		return adminControlResponse{OK: true, Data: s.server.ListIntentIDs()}
	case "recent_reports":
		return adminControlResponse{OK: true, Data: s.server.RecentReports(req.Limit)}
	case "registered_ghosts":
		return adminControlResponse{OK: true, Data: s.server.SnapshotRegisteredGhosts()}
	case "spawn_local_ghost":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := s.server.SpawnLocalGhost(ctx, req.Spawn)
		if err != nil {
			return adminControlResponse{OK: false, Error: err.Error()}
		}
		s.persistBuildlog("spawn_local_ghost", out)
		return adminControlResponse{OK: true, Data: out}
	default:
		return adminControlResponse{OK: false, Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

func mapAdminIssue(in AdminIssueRequest) IssueEnv {
	out := IssueEnv{
		IntentID:    strings.TrimSpace(in.IntentID),
		Actor:       strings.TrimSpace(in.Actor),
		TargetScope: strings.TrimSpace(in.TargetScope),
		Objective:   strings.TrimSpace(in.Objective),
		CommandPlan: make([]IssueCommand, 0, len(in.CommandPlan)),
	}
	for i := range in.CommandPlan {
		step := in.CommandPlan[i]
		out.CommandPlan = append(out.CommandPlan, IssueCommand{
			GhostID:      strings.TrimSpace(step.GhostID),
			SeedSelector: strings.TrimSpace(step.SeedSelector),
			Operation:    strings.TrimSpace(step.Operation),
			Args:         copyArgs(step.Args),
			Blocking:     step.Blocking,
		})
	}
	return out
}

func writeAdminControlResponse(w io.Writer, resp adminControlResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	_, err = w.Write(payload)
	return err
}
