package mirage

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/activity"
	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
)

// Operator parity endpoints (docs/architecture/definitions/api_v1.toml, 2026-11-25 amendment):
// remote deploy, dependency proposals, Ghost reads through Mirage, and the activity read. Deploy
// and dependency routes are admin actions and go through serveAPIv1Action; the reads below have no
// admin action of their own and are authorized as the admin read the contract names.

const (
	// Bound for one Ghost read behind /api/v1/ghosts/{ghost_id}/...
	apiV1GhostReadTimeout = 5 * time.Second
	// Audit action names for reads that have no Mirage admin action.
	apiV1GhostStatusAction       = "ghost_status"
	apiV1GhostEventsAction       = "ghost_recent_events"
	apiV1GhostVerificationAction = "ghost_verification"
	apiV1ActivityAction          = "activity"
	apiV1DeployTargetsAction     = "deploy_targets"
)

// Registers the amendment's routes on the api/v1 mux.
func (s *Service) registerAPIv1Parity(mux *http.ServeMux, route func(pattern string, ep apiV1Endpoint)) {
	route("POST /api/v1/ghosts/deploy", apiV1Endpoint{action: "deploy_ghost", privileged: true, build: apiV1BuildDeploy, shape: apiV1ShapeDeploy})
	route("GET /api/v1/dependencies/proposals", apiV1Endpoint{action: "list_dependency_proposals", shape: apiV1ShapeProposals})
	route("GET /api/v1/dependencies/proposals/{proposal_id}", apiV1Endpoint{action: "list_dependency_proposals", build: apiV1BuildProposalID, shape: apiV1ShapeProposal})
	route("POST /api/v1/dependencies/proposals", apiV1Endpoint{action: "propose_dependencies", build: apiV1BuildPropose, shape: apiV1ShapeProposal})
	route("POST /api/v1/dependencies/proposals/{proposal_id}/approve", apiV1Endpoint{action: "approve_dependencies", privileged: true, build: apiV1BuildApprove, shape: apiV1ShapeProposal})
	route("POST /api/v1/dependencies/proposals/{proposal_id}/execute", apiV1Endpoint{action: "execute_dependencies", privileged: true, build: apiV1BuildProposalID, shape: apiV1ShapeProposal})

	read := func(pattern string, auditAction string, authorizeAs string, fn func(r *http.Request) (any, error)) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
			s.serveAPIv1Read(w, r, auditAction, authorizeAs, fn)
		})
	}
	read("GET /api/v1/ghosts/deploy-targets", apiV1DeployTargetsAction, "routing_table", s.apiV1DeployTargets)
	read("GET /api/v1/ghosts/{ghost_id}/status", apiV1GhostStatusAction, "registered_ghosts", s.apiV1GhostStatus)
	read("GET /api/v1/ghosts/{ghost_id}/events", apiV1GhostEventsAction, "registered_ghosts", s.apiV1GhostEvents)
	read("GET /api/v1/ghosts/{ghost_id}/verification", apiV1GhostVerificationAction, "registered_ghosts", s.apiV1GhostVerification)
	read("GET /api/v1/activity", apiV1ActivityAction, "recent_reports", apiV1Activity)
}

// One read with no admin action behind it: authorize as the admin read the contract names, run
// fn, audit under the read's own name, and write the envelope.
func (s *Service) serveAPIv1Read(w http.ResponseWriter, r *http.Request, auditAction string, authorizeAs string, fn func(r *http.Request) (any, error)) {
	call := apiV1NewCall(r)
	if err := (adminauth.Policy{Endpoint: adminauth.MirageAdmin}).Authorize(call.peer, authorizeAs); err != nil {
		resp := failAdmin(err)
		adminauth.Audit("mirage", call.peer, auditAction, adminauth.DecisionDeny, resp.ErrorCode)
		apiV1WriteFailure(w, call, resp)
		return
	}
	data, err := fn(r)
	if err != nil {
		resp := failAdmin(err)
		adminauth.Audit("mirage", call.peer, auditAction, adminauth.DecisionAllow, resp.ErrorCode)
		apiV1WriteFailure(w, call, resp)
		return
	}
	adminauth.Audit("mirage", call.peer, auditAction, adminauth.DecisionAllow, 0)
	apiV1WriteSuccess(w, call, data)
}

// Ghost admin client for the {ghost_id} path value; unknown when Mirage has no admin route for it.
func (s *Service) apiV1GhostClient(r *http.Request) (*GhostControlClient, error) {
	ghostID := strings.TrimSpace(r.PathValue("ghost_id"))
	s.adminGhostMu.RLock()
	addr := s.adminGhostAddrs[ghostID]
	s.adminGhostMu.RUnlock()
	if ghostID == "" || strings.TrimSpace(addr) == "" {
		return nil, fmt.Errorf("%w: no admin route for ghost %q", errAPIv1UnknownID, ghostID)
	}
	return s.newGhostControlClient(ghostID, addr), nil
}

// Optional positive ?limit=; 0 when absent, which keeps the Ghost's own default.
func apiV1QueryLimit(r *http.Request) (int, error) {
	var req adminControlRequest
	if err := apiV1BuildLimit(r, &req); err != nil {
		return 0, err
	}
	return req.Limit, nil
}

func (s *Service) apiV1GhostStatus(r *http.Request) (any, error) {
	client, err := s.apiV1GhostClient(r)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiV1GhostReadTimeout)
	defer cancel()
	st, err := client.Status(ctx)
	if err != nil {
		return nil, err
	}
	return apiV1GhostStatusShape{GhostID: st.GhostID, Phase: st.Phase, SeedCount: st.SeedCount}, nil
}

func (s *Service) apiV1GhostEvents(r *http.Request) (any, error) {
	limit, err := apiV1QueryLimit(r)
	if err != nil {
		return nil, err
	}
	client, err := s.apiV1GhostClient(r)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiV1GhostReadTimeout)
	defer cancel()
	events, err := client.RecentEvents(ctx, limit)
	if err != nil {
		return nil, err
	}
	out := make([]apiV1GhostEvent, 0, len(events))
	for _, e := range events {
		out = append(out, apiV1GhostEvent(e))
	}
	return map[string][]apiV1GhostEvent{"events": out}, nil
}

func (s *Service) apiV1GhostVerification(r *http.Request) (any, error) {
	limit, err := apiV1QueryLimit(r)
	if err != nil {
		return nil, err
	}
	client, err := s.apiV1GhostClient(r)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiV1GhostReadTimeout)
	defer cancel()
	records, err := client.Verification(ctx, limit)
	if err != nil {
		return nil, err
	}
	if records == nil {
		records = []GhostVerificationRecord{}
	}
	return map[string][]GhostVerificationRecord{"records": records}, nil
}

// Configured deploy targets: identity and address only; credentials, key paths and SSH details
// stay on the Mirage host (admin_security.toml [deploy] key_transfer).
func (s *Service) apiV1DeployTargets(_ *http.Request) (any, error) {
	out := make([]apiV1DeployTarget, 0, len(s.cfg.GhostManifests))
	for i := range s.cfg.GhostManifests {
		m := s.cfg.GhostManifests[i]
		target := apiV1DeployTarget{GhostID: strings.TrimSpace(m.GhostID), Host: strings.TrimSpace(m.Host)}
		// Same derivation as deploy_ghost, including its default admin port.
		listen := strings.TrimSpace(m.AdminListen)
		if listen == "" {
			listen = "0.0.0.0:7010"
		}
		if target.Host != "" {
			target.AdminAddr = fmt.Sprintf("%s:%s", target.Host, extractPort(listen))
		}
		out = append(out, target)
	}
	return map[string][]apiV1DeployTarget{"targets": out}, nil
}

// Recent audit and custody records of this process, oldest first (api_v1.toml [parity.activity]).
func apiV1Activity(r *http.Request) (any, error) {
	limit, err := apiV1QueryLimit(r)
	if err != nil {
		return nil, err
	}
	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	if kind != "" && kind != activity.KindAudit && kind != activity.KindCustody {
		return nil, fmt.Errorf("%w: kind must be audit or custody", errAdminInvalidRequest)
	}
	return map[string][]activity.Record{"records": activity.Recent(limit, kind)}, nil
}

// apiV1GhostStatusShape is one Ghost's lifecycle status (api_v1.toml [shapes.ghost_status]).
type apiV1GhostStatusShape struct {
	GhostID   string `json:"ghost_id"`
	Phase     string `json:"phase"`
	SeedCount int    `json:"seed_count"`
}

// apiV1GhostEvent is one terminal event a Ghost recorded (api_v1.toml [shapes.ghost_events]).
type apiV1GhostEvent struct {
	EventID     string `json:"event_id"`
	CommandID   string `json:"command_id"`
	IntentID    string `json:"intent_id"`
	GhostID     string `json:"ghost_id"`
	SeedID      string `json:"seed_id"`
	Outcome     string `json:"outcome"`
	TimestampMS uint64 `json:"timestamp_ms"`
}

// apiV1DeployTarget is one configured deploy target (api_v1.toml [shapes.deploy_targets]).
type apiV1DeployTarget struct {
	GhostID   string `json:"ghost_id"`
	Host      string `json:"host"`
	AdminAddr string `json:"admin_addr"`
}

func apiV1BuildDeploy(r *http.Request, req *adminControlRequest) error {
	if err := apiV1DecodeBody(r, &req.Deploy); err != nil {
		return err
	}
	if strings.TrimSpace(req.Deploy.GhostID) == "" {
		return fmt.Errorf("%w: ghost_id required", errAdminInvalidRequest)
	}
	return nil
}

func apiV1BuildProposalID(r *http.Request, req *adminControlRequest) error {
	req.Dependency.ProposalID = strings.TrimSpace(r.PathValue("proposal_id"))
	return nil
}

// Body is { "intent": signed_intent }; the signature was made by operator tooling.
func apiV1BuildPropose(r *http.Request, req *adminControlRequest) error {
	var body struct {
		Intent *approval.SignedIntent `json:"intent"`
	}
	if err := apiV1DecodeBody(r, &body); err != nil {
		return err
	}
	if body.Intent == nil {
		return fmt.Errorf("%w: intent required", errAdminInvalidRequest)
	}
	req.Dependency.Intent = body.Intent
	return nil
}

// Body is { "approval": signed_approval }; its proposal_id must be the one in the path.
func apiV1BuildApprove(r *http.Request, req *adminControlRequest) error {
	var body struct {
		Approval *approval.Signed `json:"approval"`
	}
	if err := apiV1DecodeBody(r, &body); err != nil {
		return err
	}
	if body.Approval == nil {
		return fmt.Errorf("%w: approval required", errAdminInvalidRequest)
	}
	id := strings.TrimSpace(r.PathValue("proposal_id"))
	if body.Approval.Payload.ProposalID != id {
		return fmt.Errorf("%w: approval is for proposal %q, path names %q", errAdminInvalidRequest, body.Approval.Payload.ProposalID, id)
	}
	req.Dependency.ProposalID = id
	req.Dependency.Approval = body.Approval
	return nil
}

func apiV1ShapeDeploy(_ adminControlRequest, data any) (any, error) {
	out, ok := data.(DeployGhostResult)
	if !ok {
		return nil, apiV1BadData("deploy_ghost", data)
	}
	return out, nil
}

// One proposal: propose, approve and execute return it directly; the by-id read returns a list of one.
func apiV1ShapeProposal(_ adminControlRequest, data any) (any, error) {
	switch v := data.(type) {
	case DependencyProposal:
		return v, nil
	case []DependencyProposal:
		if len(v) == 1 {
			return v[0], nil
		}
	}
	return nil, apiV1BadData("dependency proposal", data)
}

func apiV1ShapeProposals(_ adminControlRequest, data any) (any, error) {
	list, ok := data.([]DependencyProposal)
	if !ok {
		return nil, apiV1BadData("list_dependency_proposals", data)
	}
	if list == nil {
		list = []DependencyProposal{}
	}
	return map[string][]DependencyProposal{"proposals": list}, nil
}
