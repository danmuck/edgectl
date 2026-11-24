package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"sort"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/protocol/schema"
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

// AdminIssueStage defines one stage containing ordered commands in a mirage issue.
type AdminIssueStage struct {
	ID       string              `json:"id"`
	Commands []AdminIssueCommand `json:"commands"`
	Barrier  bool                `json:"barrier"`
}

// AdminIssueRequest defines one issue ingest request for mirage admin controls.
type AdminIssueRequest struct {
	IntentID         string            `json:"intent_id"`
	Actor            string            `json:"actor"`
	TargetScope      string            `json:"target_scope"`
	Objective        string            `json:"objective"`
	SeedDependencies []string          `json:"seed_dependencies,omitempty"`
	Stages           []AdminIssueStage `json:"stages"`
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

// AdminAttachGhostResponse captures one remote ghost admin binding.
type AdminAttachGhostResponse struct {
	GhostID   string `json:"ghost_id"`
	AdminAddr string `json:"admin_addr"`
}

type adminControlRequest struct {
	Action         string             `json:"action"`
	Limit          int                `json:"limit,omitempty"`
	IntentID       string             `json:"intent_id,omitempty"`
	Issue          AdminIssueRequest  `json:"issue,omitempty"`
	Spawn          SpawnGhostRequest  `json:"spawn,omitempty"`
	Deploy         DeployGhostRequest `json:"deploy,omitempty"`
	GhostAdminAddr string             `json:"ghost_admin_addr,omitempty"`
	// Dependency proposal parameters (propose/approve/execute/list_dependency_*).
	Dependency AdminDependencyRequest `json:"dependency,omitempty"`
}

type adminControlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	// Canonical wire code (errors.toml 1000-1500) when OK is false.
	ErrorCode uint32 `json:"error_code,omitempty"`
	Data      any    `json:"data,omitempty"`
}

// Admin request is malformed, missing a required parameter, or names no known action.
var errAdminInvalidRequest = errors.New("mirage.admin: invalid request")

// Canonical wire code for a Mirage admin failure (errors.toml [wire_codes]). Codes reported by a
// Ghost admin endpoint pass through; orchestration input errors are semantic; protocol errors
// fall back to session.ErrorCode.
func adminErrorCode(err error) uint32 {
	var ghostErr *GhostAdminError
	if errors.As(err, &ghostErr) && ghostErr.Code != 0 {
		return ghostErr.Code
	}
	if errors.Is(err, adminauth.ErrAuthRequired) || errors.Is(err, adminauth.ErrAuthorizationDenied) {
		return schema.ErrCodeTransportFailure
	}
	switch {
	case errors.Is(err, errAdminInvalidRequest),
		errors.Is(err, approval.ErrInvalidApproval),
		errors.Is(err, approval.ErrApprovalExpired),
		errors.Is(err, approval.ErrSeparationOfDuties),
		errors.Is(err, approval.ErrStepsMismatch),
		errors.Is(err, ErrInvalidDependencyProposal),
		errors.Is(err, ErrDependencyProposalNotFound),
		errors.Is(err, ErrDependencyProposalState),
		errors.Is(err, ErrInvalidIssue),
		errors.Is(err, ErrIntentNotFound),
		errors.Is(err, ErrTargetGhostRequired):
		return schema.ErrCodeSemanticValidation
	}
	return session.ErrorCode(err)
}

// Failure response carrying the error text and its canonical wire code.
func failAdmin(err error) adminControlResponse {
	return adminControlResponse{OK: false, Error: err.Error(), ErrorCode: adminErrorCode(err)}
}

// Failure response for a malformed or incomplete admin request.
func failAdminRequest(format string, args ...any) adminControlResponse {
	return failAdmin(fmt.Errorf("%w: "+format, append([]any{errAdminInvalidRequest}, args...)...))
}

// serveAdminControl exposes a TCP JSON request/response endpoint for Mirage control.
func (s *Service) serveAdminControl(ctx context.Context, addr string) error {
	// mTLS per admin_security.toml [transport]; plaintext only on loopback outside production.
	ln, err := adminauth.Listen(addr, s.cfg.AdminTLS, s.selfPrincipal(), s.productionMode())
	if err != nil {
		return err
	}
	defer ln.Close()
	logs.Warnf("mirage.admin listening addr=%q", ln.Addr().String())

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
	peer, err := adminauth.PeerFromConn(conn, 5*time.Second)
	if err != nil {
		logs.Warnf("mirage.admin handshake rejected component=mirage peer=%q error_code=%d err=%q", remote, schema.ErrCodeTransportFailure, err.Error())
		return
	}
	active := s.adminClientCount.Add(1)
	logs.Warnf("mirage.admin client connected remote=%q active_clients=%d", remote, active)
	defer func() {
		remaining := s.adminClientCount.Add(-1)
		logs.Warnf("mirage.admin client disconnected remote=%q active_clients=%d", remote, remaining)
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
			_ = writeAdminControlResponse(conn, failAdminRequest("%v", err))
			continue
		}
		// Default-deny authorization by principal, then actor binding, before dispatch
		// (admin_security.toml [authorization]).
		var resp adminControlResponse
		decision := adminauth.DecisionAllow
		if err := s.authorizeAdminRequest(peer, &req); err != nil {
			resp = failAdmin(err)
			decision = adminauth.DecisionDeny
		} else {
			resp = s.handleAdminControlRequest(req)
		}
		adminauth.Audit("mirage", peer, req.Action, decision, resp.ErrorCode)
		if err := writeAdminControlResponse(conn, resp); err != nil {
			logs.Warnf("mirage.admin write err=%v", err)
			return
		}
	}
}

// handleAdminControlRequest routes one admin action to Mirage runtime methods.
func (s *Service) handleAdminControlRequest(req adminControlRequest) adminControlResponse {
	if resp, ok := s.handleDependencyAdminAction(strings.TrimSpace(req.Action), req.Dependency); ok {
		return resp
	}
	switch strings.TrimSpace(req.Action) {
	case "status":
		status := s.server.Status()
		status.RegisteredGhosts = len(s.SnapshotConnectedGhosts())
		return adminControlResponse{OK: true, Data: status}
	case "submit_issue":
		issue := mapAdminIssue(req.Issue)
		if err := s.server.SubmitIssue(issue); err != nil {
			return failAdmin(err)
		}
		s.persistBuildlog("submit_issue", map[string]any{
			"intent_id": issue.IntentID,
			"actor":     issue.Actor,
		})
		return adminControlResponse{OK: true}
	case "reconcile_intent":
		intentID := strings.TrimSpace(req.IntentID)
		if intentID == "" {
			return failAdminRequest("intent_id required")
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		report, err := s.server.ReconcileIntent(ctx, intentID)
		if err != nil {
			return failAdmin(err)
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
				return failAdmin(fmt.Errorf("intent_id=%s: %w", intentID, err))
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
			return failAdminRequest("intent_id required")
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
		return adminControlResponse{OK: true, Data: s.SnapshotConnectedGhosts()}
	case "routing_table":
		return adminControlResponse{OK: true, Data: s.SnapshotRoutingTable()}
	case "available_services":
		return adminControlResponse{OK: true, Data: s.SnapshotAvailableServices()}
	case "seed_catalog":
		return adminControlResponse{OK: true, Data: s.SnapshotSeedCatalog()}
	case "attach_ghost_admin":
		addr := strings.TrimSpace(req.GhostAdminAddr)
		if addr == "" {
			return failAdminRequest("ghost_admin_addr required")
		}
		out, err := s.attachGhostAdmin("", addr)
		if err != nil {
			return failAdmin(err)
		}
		logs.Warnf("mirage.admin attached ghost route ghost_id=%q addr=%q", out.GhostID, out.AdminAddr)
		return adminControlResponse{OK: true, Data: out}
	case "spawn_local_ghost":
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		out, err := s.server.SpawnLocalGhost(ctx, req.Spawn)
		if err != nil {
			return failAdmin(err)
		}
		if err := s.bindGhostToMirage(out.GhostID, out.AdminAddr); err != nil {
			return failAdmin(err)
		}
		s.bindGhostAdmin(out.GhostID, out.AdminAddr)
		logs.Warnf("mirage.admin spawned local ghost ghost_id=%q addr=%q", out.GhostID, out.AdminAddr)
		return adminControlResponse{OK: true, Data: out}
	case "deploy_ghost":
		deployer := NewSSHDeployer()
		deployer.adminFrameAuthToken = s.cfg.GhostAdminFrameAuthToken
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		result, err := deployer.Deploy(ctx, req.Deploy)
		if err != nil {
			return failAdmin(err)
		}
		// Auto-attach the newly deployed ghost via admin routing.
		if result.AdminAddr != "" {
			attachCtx, attachCancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer attachCancel()
			// Wait briefly for process startup before attempting attach.
			select {
			case <-attachCtx.Done():
			case <-time.After(2 * time.Second):
			}
			out, attachErr := s.attachGhostAdmin(result.GhostID, result.AdminAddr)
			if attachErr != nil {
				logs.Warnf("mirage.admin deploy_ghost auto-attach failed ghost_id=%q addr=%q err=%v",
					result.GhostID, result.AdminAddr, attachErr)
				result.Message += "; auto-attach failed: " + attachErr.Error()
			} else {
				logs.Warnf("mirage.admin deploy_ghost attached ghost_id=%q addr=%q", out.GhostID, out.AdminAddr)
				result.Status = "deployed_and_attached"
			}
		}
		s.persistBuildlog("deploy_ghost", map[string]any{
			"ghost_id": result.GhostID,
			"host":     result.Host,
			"status":   result.Status,
		})
		return adminControlResponse{OK: true, Data: result}
	default:
		return failAdminRequest("unknown action: %s", req.Action)
	}
}

func (s *Service) attachGhostAdmin(expectedGhostID string, adminAddr string) (AdminAttachGhostResponse, error) {
	addr, err := normalizeGhostAdminAddr(adminAddr)
	if err != nil {
		return AdminAttachGhostResponse{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	client := s.newGhostControlClient(expectedGhostID, addr)
	status, err := client.Status(ctx)
	if err != nil {
		return AdminAttachGhostResponse{}, err
	}
	ghostID := strings.TrimSpace(status.GhostID)
	if ghostID == "" {
		return AdminAttachGhostResponse{}, fmt.Errorf("ghost status missing ghost_id")
	}
	// With mTLS the certificate principal, not the self-reported status, is authoritative.
	if s.cfg.AdminTLS.Enabled() {
		if peer := client.PeerPrincipal(); peer.ID != ghostID {
			return AdminAttachGhostResponse{}, fmt.Errorf("ghost_id mismatch certificate=%q reported=%q", peer.ID, ghostID)
		}
	}
	expected := strings.TrimSpace(expectedGhostID)
	if expected != "" && ghostID != expected {
		return AdminAttachGhostResponse{}, fmt.Errorf(
			"ghost_id mismatch expected=%q reported=%q",
			expected,
			ghostID,
		)
	}
	if err := s.server.RegisterExecutor(ghostID, NewGhostAdminCommandExecutor(client)); err != nil {
		return AdminAttachGhostResponse{}, err
	}
	if err := s.bindGhostToMirage(ghostID, addr); err != nil {
		return AdminAttachGhostResponse{}, err
	}
	s.bindGhostAdmin(ghostID, addr)
	return AdminAttachGhostResponse{GhostID: ghostID, AdminAddr: addr}, nil
}

// mapAdminIssue converts a wire-format admin issue request to an internal IssueEnv.
func mapAdminIssue(in AdminIssueRequest) IssueEnv {
	out := IssueEnv{
		IntentID:         strings.TrimSpace(in.IntentID),
		Actor:            strings.TrimSpace(in.Actor),
		TargetScope:      strings.TrimSpace(in.TargetScope),
		Objective:        strings.TrimSpace(in.Objective),
		SeedDependencies: normalizeStringList(in.SeedDependencies),
		Stages:           make([]IssueStage, 0, len(in.Stages)),
	}
	for i := range in.Stages {
		src := in.Stages[i]
		stage := IssueStage{
			ID:       strings.TrimSpace(src.ID),
			Barrier:  src.Barrier,
			Commands: make([]IssueCommand, 0, len(src.Commands)),
		}
		for j := range src.Commands {
			stage.Commands = append(stage.Commands, mapAdminIssueCommand(src.Commands[j]))
		}
		out.Stages = append(out.Stages, stage)
	}
	return out
}

// mapAdminIssueCommand converts one wire-format command to internal IssueCommand.
func mapAdminIssueCommand(in AdminIssueCommand) IssueCommand {
	return IssueCommand{
		GhostID:      strings.TrimSpace(in.GhostID),
		SeedSelector: strings.TrimSpace(in.SeedSelector),
		Operation:    strings.TrimSpace(in.Operation),
		Args:         copyArgs(in.Args),
		Blocking:     in.Blocking,
	}
}

func normalizeStringList(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for i := range in {
		v := strings.TrimSpace(in[i])
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
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

// Authorizes one admin request for peer and binds request actor fields to the peer principal:
// operators only (admin_security.toml [authorization.mirage_admin]); actor fields must be empty or
// equal the caller and are filled from the certificate when empty.
func (s *Service) authorizeAdminRequest(peer adminauth.Peer, req *adminControlRequest) error {
	if err := (adminauth.Policy{Endpoint: adminauth.MirageAdmin}).Authorize(peer, req.Action); err != nil {
		return err
	}
	var err error
	switch strings.TrimSpace(req.Action) {
	case "submit_issue":
		req.Issue.Actor, err = adminauth.BindActor(peer, req.Issue.Actor)
	case "propose_dependencies", "approve_dependencies", "execute_dependencies":
		req.Dependency.Actor, err = adminauth.BindActor(peer, req.Dependency.Actor)
	}
	return err
}
