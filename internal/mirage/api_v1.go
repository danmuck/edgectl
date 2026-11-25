package mirage

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/schema"
	logs "github.com/danmuck/smplog"
)

// Northbound api/v1 (docs/architecture/definitions/api_v1.toml). Every endpoint is a thin HTTP
// form of one existing admin action: it builds the same admin request, runs the same authorization
// and dispatch as the admin endpoint, and re-encodes the result in the contract's shapes.

const (
	// Request body cap (api_v1.toml [transport] request_body_limit_bytes).
	apiV1BodyLimit = 16 << 20
	// Optional correlation header (api_v1.toml [envelope.request_id]).
	apiV1RequestIDHeader = "X-Request-Id"
	// Audit action name for the execution read; it is a Ghost admin action Mirage performs.
	apiV1ExecutionAction = "execution_by_command_id"
	// Bound for the Ghost read behind GET /api/v1/executions/{command_id}.
	apiV1ExecutionTimeout = 5 * time.Second
)

// Unknown api/v1 path or method.
var errAPIv1NotFound = errors.New("mirage.api: no such endpoint")

// No intent or Ghost record matches the requested id.
var errAPIv1UnknownID = errors.New("mirage.api: not found")

// apiV1Error is the error object of a failed response (api_v1.toml [envelope.error_fields]).
type apiV1Error struct {
	Class     string `json:"class"`
	Code      uint32 `json:"code"`
	Message   string `json:"message"`
	Retryable bool   `json:"retryable"`
}

// apiV1Envelope is the response envelope for every api/v1 reply (api_v1.toml [envelope]).
type apiV1Envelope struct {
	OK          bool        `json:"ok"`
	RequestID   string      `json:"request_id"`
	TraceID     string      `json:"trace_id"`
	TimestampMS int64       `json:"timestamp_ms"`
	Data        any         `json:"data,omitempty"`
	Error       *apiV1Error `json:"error,omitempty"`
}

// apiV1Call carries one request's correlation and caller through a handler.
type apiV1Call struct {
	requestID string
	intentID  string
	peer      adminauth.Peer
}

// apiV1Endpoint binds one route to its admin action.
type apiV1Endpoint struct {
	// Admin action the endpoint maps to (api_v1.toml [endpoints.*] action).
	action string
	// Fills the admin request from the HTTP request; nil when the action takes no parameters.
	build func(r *http.Request, req *adminControlRequest) error
	// Converts the admin action's data to the contract shape.
	shape func(req adminControlRequest, data any) (any, error)
}

// Opens the api/v1 listener under the admin transport rules (mTLS when configured; plaintext only
// on loopback outside production) and serves until ctx ends.
func (s *Service) serveAPIv1(ctx context.Context, addr string) error {
	ln, err := adminauth.Listen(addr, s.cfg.AdminTLS, s.selfPrincipal(), s.productionMode())
	if err != nil {
		return err
	}
	logs.Infof("mirage.api listening addr=%q", ln.Addr().String())
	return s.ServeAPIv1(ctx, ln)
}

// ServeAPIv1 answers api/v1 requests on ln until ctx ends or ln fails; ln comes from
// adminauth.Listen so transport rules are already enforced. Closes ln on return.
func (s *Service) ServeAPIv1(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{
		Handler:           s.apiV1Handler(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		// Above the longest mapped action (reconcile_all runs intents in sequence).
		WriteTimeout: 2 * time.Minute,
		IdleTimeout:  5 * time.Minute,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	err := srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) || ctx.Err() != nil {
		return nil
	}
	return err
}

// Route table for api/v1 (api_v1.toml [endpoints]); anything else gets a contract error envelope.
func (s *Service) apiV1Handler() http.Handler {
	mux := http.NewServeMux()
	route := func(pattern string, ep apiV1Endpoint) {
		mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) { s.serveAPIv1Action(w, r, ep) })
	}
	route("GET /api/v1/status", apiV1Endpoint{action: "status", shape: apiV1ShapeStatus})
	route("POST /api/v1/intents", apiV1Endpoint{action: "submit_issue", build: apiV1BuildIssue, shape: apiV1ShapeIntentRef})
	route("GET /api/v1/intents", apiV1Endpoint{action: "list_intents", shape: apiV1ShapeIntentIDs})
	route("GET /api/v1/intents/{intent_id}", apiV1Endpoint{action: "snapshot_intent", build: apiV1BuildIntentID, shape: apiV1ShapeSnapshot})
	route("POST /api/v1/intents/{intent_id}/reconcile", apiV1Endpoint{action: "reconcile_intent", build: apiV1BuildIntentID, shape: apiV1ShapeReport})
	route("POST /api/v1/reconcile", apiV1Endpoint{action: "reconcile_all", shape: apiV1ShapeReconcileAll})
	route("GET /api/v1/reports", apiV1Endpoint{action: "recent_reports", build: apiV1BuildLimit, shape: apiV1ShapeReports})
	route("GET /api/v1/ghosts", apiV1Endpoint{action: "registered_ghosts", shape: apiV1ShapeGhosts})
	route("GET /api/v1/routing", apiV1Endpoint{action: "routing_table", shape: apiV1ShapeRoutes})
	route("GET /api/v1/services", apiV1Endpoint{action: "available_services", shape: apiV1ShapeServices})
	route("GET /api/v1/seeds", apiV1Endpoint{action: "seed_catalog", shape: apiV1ShapeSeedCatalogs})
	route("POST /api/v1/ghosts/attach", apiV1Endpoint{action: "attach_ghost_admin", build: apiV1BuildAttach, shape: apiV1ShapeRouteRef})
	route("POST /api/v1/ghosts/spawn-local", apiV1Endpoint{action: "spawn_local_ghost", build: apiV1BuildSpawn, shape: apiV1ShapeSpawn})
	mux.HandleFunc("GET /api/v1/executions/{command_id}", s.serveAPIv1Execution)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		call := apiV1NewCall(r)
		resp := failAdmin(fmt.Errorf("%w: %s %s", errAPIv1NotFound, r.Method, r.URL.Path))
		adminauth.Audit("mirage", call.peer, "unknown", adminauth.DecisionDeny, resp.ErrorCode)
		apiV1WriteFailure(w, call, resp)
	})
	return mux
}

// One mapped admin action: decode, authorize as the admin endpoint does, dispatch through the
// admin dispatcher, audit, and encode the contract shape.
func (s *Service) serveAPIv1Action(w http.ResponseWriter, r *http.Request, ep apiV1Endpoint) {
	call := apiV1NewCall(r)
	req := adminControlRequest{Action: ep.action}
	if ep.build != nil {
		if err := ep.build(r, &req); err != nil {
			resp := failAdmin(err)
			adminauth.Audit("mirage", call.peer, ep.action, adminauth.DecisionDeny, resp.ErrorCode)
			apiV1WriteFailure(w, call, resp)
			return
		}
	}
	call.intentID = strings.TrimSpace(req.IntentID)
	if ep.action == "submit_issue" {
		call.intentID = strings.TrimSpace(req.Issue.IntentID)
	}
	if err := s.authorizeAdminRequest(call.peer, &req); err != nil {
		resp := failAdmin(err)
		adminauth.Audit("mirage", call.peer, ep.action, adminauth.DecisionDeny, resp.ErrorCode)
		apiV1WriteFailure(w, call, resp)
		return
	}
	resp := s.handleAdminControlRequest(req)
	adminauth.Audit("mirage", call.peer, ep.action, adminauth.DecisionAllow, resp.ErrorCode)
	if !resp.OK {
		apiV1WriteFailure(w, call, resp)
		return
	}
	data, err := ep.shape(req, resp.Data)
	if err != nil {
		apiV1WriteFailure(w, call, failAdmin(err))
		return
	}
	apiV1WriteSuccess(w, call, data)
}

// Execution read: Mirage finds the Ghost that owns command_id in its planned commands and reads
// the record from that Ghost, so a client never calls a Ghost (api_v1.toml [boundary] ghost_access).
// Authorized as snapshot_intent (api_v1.toml [endpoints.get_execution]).
func (s *Service) serveAPIv1Execution(w http.ResponseWriter, r *http.Request) {
	call := apiV1NewCall(r)
	commandID := strings.TrimSpace(r.PathValue("command_id"))
	fail := func(decision string, err error) {
		resp := failAdmin(err)
		adminauth.Audit("mirage", call.peer, apiV1ExecutionAction, decision, resp.ErrorCode)
		apiV1WriteFailure(w, call, resp)
	}
	if err := (adminauth.Policy{Endpoint: adminauth.MirageAdmin}).Authorize(call.peer, "snapshot_intent"); err != nil {
		fail(adminauth.DecisionDeny, err)
		return
	}
	intentID, ghostID, ok := s.commandOwner(commandID)
	if !ok {
		fail(adminauth.DecisionAllow, fmt.Errorf("%w: command_id %q", errAPIv1UnknownID, commandID))
		return
	}
	call.intentID = intentID
	s.adminGhostMu.RLock()
	addr := s.adminGhostAddrs[ghostID]
	s.adminGhostMu.RUnlock()
	if strings.TrimSpace(addr) == "" {
		fail(adminauth.DecisionAllow, fmt.Errorf("%w: no admin route for ghost %q", errAPIv1UnknownID, ghostID))
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), apiV1ExecutionTimeout)
	defer cancel()
	exec, found, err := s.newGhostControlClient(ghostID, addr).ExecutionByCommandID(ctx, commandID)
	if err != nil {
		fail(adminauth.DecisionAllow, err)
		return
	}
	if !found {
		fail(adminauth.DecisionAllow, fmt.Errorf("%w: ghost %q holds no execution for command_id %q", errAPIv1UnknownID, ghostID, commandID))
		return
	}
	adminauth.Audit("mirage", call.peer, apiV1ExecutionAction, adminauth.DecisionAllow, 0)
	apiV1WriteSuccess(w, call, apiV1ShapeExecution(exec))
}

// Intent and Ghost that own a planned command_id, from Mirage desired state.
func (s *Service) commandOwner(commandID string) (intentID string, ghostID string, ok bool) {
	if commandID == "" {
		return "", "", false
	}
	for _, id := range s.server.ListIntentIDs() {
		snap, found := s.server.SnapshotIntent(id)
		if !found {
			continue
		}
		for i := range snap.Desired.Commands {
			cmd := snap.Desired.Commands[i].Command
			if cmd.CommandID == commandID {
				return id, cmd.GhostID, true
			}
		}
	}
	return "", "", false
}

// Caller and correlation for one request: principal from the verified client certificate,
// request_id from the header or generated.
func apiV1NewCall(r *http.Request) apiV1Call {
	call := apiV1Call{peer: adminauth.Peer{Addr: r.RemoteAddr}}
	if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 {
		if p, err := identity.PrincipalFromCert(r.TLS.PeerCertificates[0]); err == nil {
			call.peer.Principal = p
			call.peer.Authenticated = true
		}
	}
	call.requestID = strings.TrimSpace(r.Header.Get(apiV1RequestIDHeader))
	if call.requestID == "" || len(call.requestID) > 128 {
		call.requestID = apiV1NewRequestID()
	}
	return call
}

// Generated request id for callers that sent none.
func apiV1NewRequestID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("req.%d", time.Now().UnixNano())
	}
	return "req." + hex.EncodeToString(b[:])
}

// trace.<intent_id> when the request concerns one intent, matching custody logging; otherwise
// derived from the request id (api_v1.toml [envelope.success] trace_id).
func (c apiV1Call) traceID() string {
	if c.intentID != "" {
		return "trace." + c.intentID
	}
	return "trace." + c.requestID
}

func apiV1WriteSuccess(w http.ResponseWriter, call apiV1Call, data any) {
	apiV1Write(w, http.StatusOK, apiV1Envelope{
		OK:          true,
		RequestID:   call.requestID,
		TraceID:     call.traceID(),
		TimestampMS: time.Now().UnixMilli(),
		Data:        data,
	})
}

// Failure envelope with the admin action's wire code and the HTTP status the contract maps it to.
func apiV1WriteFailure(w http.ResponseWriter, call apiV1Call, resp adminControlResponse) {
	status := apiV1HTTPStatus(resp)
	apiV1Write(w, status, apiV1Envelope{
		OK:          false,
		RequestID:   call.requestID,
		TraceID:     call.traceID(),
		TimestampMS: time.Now().UnixMilli(),
		Error: &apiV1Error{
			Class:     apiV1ErrorClass(resp.ErrorCode),
			Code:      resp.ErrorCode,
			Message:   resp.Error,
			Retryable: apiV1Retryable(status, resp),
		},
	})
}

func apiV1Write(w http.ResponseWriter, status int, env apiV1Envelope) {
	payload, err := json.Marshal(env)
	if err != nil {
		logs.Warnf("mirage.api encode err=%v", err)
		http.Error(w, `{"ok":false}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set(apiV1RequestIDHeader, env.RequestID)
	w.WriteHeader(status)
	if _, err := w.Write(append(payload, '\n')); err != nil {
		logs.Warnf("mirage.api write err=%v", err)
	}
}

// Retryable only when Mirage could not reach a Ghost at all: nothing ran, so nothing is partial.
// A failure the Ghost itself reported is not retried blindly.
func apiV1Retryable(status int, resp adminControlResponse) bool {
	var ghostErr *GhostAdminError
	if errors.As(resp.cause, &ghostErr) {
		return false
	}
	return status == http.StatusBadGateway && resp.ErrorCode == schema.ErrCodeTransportFailure
}

// HTTP status for a failed admin response (api_v1.toml [envelope.http_status]).
func apiV1HTTPStatus(resp adminControlResponse) int {
	err := resp.cause
	switch {
	case errors.Is(err, adminauth.ErrAuthRequired),
		errors.Is(err, adminauth.ErrAuthorizationDenied),
		errors.Is(err, ErrIntentActorMismatch):
		return http.StatusForbidden
	case errors.Is(err, ErrIntentNotFound),
		errors.Is(err, errAPIv1UnknownID),
		errors.Is(err, errAPIv1NotFound):
		return http.StatusNotFound
	}
	switch resp.ErrorCode {
	case schema.ErrCodeSemanticValidation:
		return http.StatusBadRequest
	case schema.ErrCodeTransportFailure, schema.ErrCodeRuntimeExecFailure:
		return http.StatusBadGateway
	}
	return http.StatusInternalServerError
}

// Error class for a wire code (errors.toml [classes], [wire_codes]).
func apiV1ErrorClass(code uint32) string {
	switch {
	case code >= 1000 && code < 1100:
		return "transport"
	case code >= 1100 && code < 1200:
		return "framing"
	case code >= 1200 && code < 1300:
		return "tlv_decode"
	case code >= 1300 && code < 1400:
		return "semantic"
	default:
		return "runtime"
	}
}

// Decodes a JSON request body into out; an empty body, an oversize body, unknown fields and
// trailing data are all request errors.
func apiV1DecodeBody(r *http.Request, out any) error {
	body, err := io.ReadAll(io.LimitReader(r.Body, apiV1BodyLimit+1))
	if err != nil {
		return fmt.Errorf("%w: read body: %v", errAdminInvalidRequest, err)
	}
	if len(body) > apiV1BodyLimit {
		return fmt.Errorf("%w: body exceeds %d bytes", errAdminInvalidRequest, apiV1BodyLimit)
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return fmt.Errorf("%w: %v", errAdminInvalidRequest, err)
	}
	if dec.More() {
		return fmt.Errorf("%w: trailing data after JSON body", errAdminInvalidRequest)
	}
	return nil
}

// POST /api/v1/intents: the body is one issue envelope (api_v1.toml [shapes.issue]).
func apiV1BuildIssue(r *http.Request, req *adminControlRequest) error {
	return apiV1DecodeBody(r, &req.Issue)
}

func apiV1BuildIntentID(r *http.Request, req *adminControlRequest) error {
	req.IntentID = strings.TrimSpace(r.PathValue("intent_id"))
	return nil
}

// GET /api/v1/reports?limit=N; absent keeps the recent_reports default.
func apiV1BuildLimit(r *http.Request, req *adminControlRequest) error {
	raw := strings.TrimSpace(r.URL.Query().Get("limit"))
	if raw == "" {
		return nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return fmt.Errorf("%w: limit must be a positive integer", errAdminInvalidRequest)
	}
	req.Limit = limit
	return nil
}

func apiV1BuildAttach(r *http.Request, req *adminControlRequest) error {
	var body struct {
		GhostAdminAddr string `json:"ghost_admin_addr"`
	}
	if err := apiV1DecodeBody(r, &body); err != nil {
		return err
	}
	req.GhostAdminAddr = body.GhostAdminAddr
	return nil
}

func apiV1BuildSpawn(r *http.Request, req *adminControlRequest) error {
	if err := apiV1DecodeBody(r, &req.Spawn); err != nil {
		return err
	}
	if strings.TrimSpace(req.Spawn.TargetName) == "" || strings.TrimSpace(req.Spawn.AdminAddr) == "" {
		return fmt.Errorf("%w: target_name and admin_addr required", errAdminInvalidRequest)
	}
	return nil
}
