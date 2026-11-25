package mirage

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/danmuck/edgectl/internal/adminauth"
	"github.com/danmuck/edgectl/internal/approval"
	"github.com/danmuck/edgectl/internal/identity"
	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
)

const (
	spawnGhostAction      = "spawn_ghost"
	executeEnvelopeAction = "execute_envelope"
	statusAction          = "status"
	listSeedsAction       = "list_seeds"
	listSeedCatalogAction = "list_seed_catalog"
	bindMirageAction      = "bind_mirage"
	// Ghost read of one execution record, used for the api/v1 execution read.
	executionByCommandIDAction = "execution_by_command_id"
	// Ghost reads behind /api/v1/ghosts/{ghost_id}/events and /verification.
	recentEventsAction = "recent_events"
	verificationAction = "verification"
)

type ghostControlRequest struct {
	Action       string            `json:"action"`
	Spawn        SpawnGhostRequest `json:"spawn,omitempty"`
	CommandFrame []byte            `json:"command_frame,omitempty"`
	CommandID    string            `json:"command_id,omitempty"`
	Limit        int               `json:"limit,omitempty"`
	MirageID     string            `json:"mirage_id,omitempty"`
	SessionPort  string            `json:"session_port,omitempty"`
	Install      *ghostInstallPlan `json:"install,omitempty"`
}

// Wire shape of Ghost's install_dependencies request (ghost.InstallPlanRequest).
type ghostInstallPlan struct {
	ProposalID string              `json:"proposal_id"`
	ApprovedBy string              `json:"approved_by"`
	Specs      []seeds.InstallSpec `json:"specs"`
	Approval   *approval.Signed    `json:"approval,omitempty"`
}

// Wire shape of Ghost's install_dependencies result (ghost.InstallPlanResult).
type ghostInstallPlanResult struct {
	ProposalID string                     `json:"proposal_id"`
	Succeeded  bool                       `json:"succeeded"`
	Steps      []DependencyStepResult     `json:"steps"`
	Rollback   []DependencyRollbackResult `json:"rollback"`
}

// Upper bound for one dependency plan; package installs can take minutes.
const dependencyInstallTimeout = 15 * time.Minute

type ghostAdminCommand struct {
	GhostID      string            `json:"ghost_id,omitempty"`
	CommandID    string            `json:"command_id,omitempty"`
	IntentID     string            `json:"intent_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
}

type ghostControlResponse struct {
	OK        bool            `json:"ok"`
	Error     string          `json:"error,omitempty"`
	ErrorCode uint32          `json:"error_code,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
}

// GhostAdminError is a failure reported by a Ghost admin endpoint, carrying its canonical wire code.
type GhostAdminError struct {
	Action  string
	Code    uint32
	Message string
}

func (e *GhostAdminError) Error() string {
	return fmt.Sprintf("mirage: ghost control %s failed: error_code=%d %s", e.Action, e.Code, e.Message)
}

type ghostEvent struct {
	EventID     string `json:"event_id"`
	CommandID   string `json:"command_id"`
	IntentID    string `json:"intent_id"`
	GhostID     string `json:"ghost_id"`
	SeedID      string `json:"seed_id"`
	Outcome     string `json:"outcome"`
	TimestampMS uint64 `json:"timestamp_ms"`
}

type ghostExecuteResponse struct {
	Event ghostEvent `json:"event"`
}

type ghostExecuteEnvelopeResponse struct {
	EventFrame []byte `json:"event_frame"`
}

type ghostLifecycleStatus struct {
	// GhostID accepts snake_case payloads if they are introduced later.
	GhostID string `json:"ghost_id"`
	// GhostIDLegacy accepts the current Ghost admin status shape from untagged Go structs.
	GhostIDLegacy string `json:"GhostID"`
	// Lifecycle phase and seed count, as Ghost encodes them (untagged Go field names).
	Phase     string `json:"Phase"`
	SeedCount int    `json:"SeedCount"`
}

type ghostSeedMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ghostSeedCapability struct {
	Metadata       ghostSeedMetadata      `json:"metadata"`
	Operations     []ghostOperationSpec   `json:"operations"`
	CommandCatalog []ghostCommandTemplate `json:"command_catalog"`
}

type ghostOperationSpec struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Idempotent  bool   `json:"idempotent"`
}

type ghostCommandTemplate struct {
	ID              string                `json:"id"`
	Label           string                `json:"label"`
	Description     string                `json:"description"`
	SeedSelector    string                `json:"seed_selector"`
	Operation       string                `json:"operation"`
	Args            []ghostCommandArgSpec `json:"args"`
	DefaultBlocking bool                  `json:"default_blocking"`
}

type ghostCommandArgSpec struct {
	Key          string `json:"key"`
	Prompt       string `json:"prompt"`
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value"`
	Multiline    bool   `json:"multiline"`
	Terminator   string `json:"terminator"`
}

// GhostControlClient is a TCP JSON control client for one root/local ghost admin endpoint.
type GhostControlClient struct {
	adminAddr string
	timeout   time.Duration
	// mTLS dial config (nil = plaintext); tlsErr defers credential load errors to the first call.
	tlsConfig *tls.Config
	tlsErr    error
	// Ghost principal observed on the last verified TLS handshake.
	peerMu sync.Mutex
	peer   identity.Principal
}

// WithTLS sets the mTLS dial configuration; a non-nil err fails every call.
func (c *GhostControlClient) WithTLS(cfg *tls.Config, err error) *GhostControlClient {
	c.tlsConfig = cfg
	c.tlsErr = err
	return c
}

// PeerPrincipal returns the Ghost principal verified on the most recent TLS handshake.
func (c *GhostControlClient) PeerPrincipal() identity.Principal {
	c.peerMu.Lock()
	defer c.peerMu.Unlock()
	return c.peer
}

// Records the verified server principal; used from the TLS verification callback.
func (c *GhostControlClient) setPeer(p identity.Principal) {
	c.peerMu.Lock()
	c.peer = p
	c.peerMu.Unlock()
}

// NewGhostControlClient constructs a control client bound to one ghost admin address.
func NewGhostControlClient(adminAddr string) *GhostControlClient {
	return &GhostControlClient{
		adminAddr: strings.TrimSpace(adminAddr),
		timeout:   5 * time.Second,
	}
}

// InstallDependencies sends an approved proposal's plan to Ghost "install_dependencies" and returns
// Ghost's per-step result. Uses a long per-call timeout instead of the client default.
func (c *GhostControlClient) InstallDependencies(ctx context.Context, p DependencyProposal) (DependencyExecution, error) {
	var out ghostInstallPlanResult
	if err := c.callTimeout(ctx, dependencyInstallTimeout, ghostControlRequest{
		Action: "install_dependencies",
		Install: &ghostInstallPlan{
			ProposalID: p.ID,
			ApprovedBy: p.ApprovedBy,
			Specs:      p.Steps,
			Approval:   p.Approval,
		},
	}, &out); err != nil {
		return DependencyExecution{}, err
	}
	return DependencyExecution{Succeeded: out.Succeeded, Steps: out.Steps, Rollback: out.Rollback}, nil
}

// SpawnLocalGhost calls root ghost admin "spawn_ghost" for local provisioning.
func (c *GhostControlClient) SpawnLocalGhost(ctx context.Context, req SpawnGhostRequest) (SpawnGhostResult, error) {
	var out SpawnGhostResult
	if err := c.call(ctx, ghostControlRequest{
		Action: spawnGhostAction,
		Spawn:  req,
	}, &out); err != nil {
		return SpawnGhostResult{}, err
	}
	return out, nil
}

// ExecuteAdminCommand calls ghost "execute_envelope" and returns one terminal event payload.
func (c *GhostControlClient) ExecuteAdminCommand(ctx context.Context, command ghostAdminCommand) (session.Event, error) {
	ghostID := strings.TrimSpace(command.GhostID)
	if ghostID == "" {
		ghostID = "ghost.local"
	}
	cmdFrame, err := session.EncodeCommandFrame(1, session.Command{
		CommandID:    strings.TrimSpace(command.CommandID),
		IntentID:     strings.TrimSpace(command.IntentID),
		GhostID:      ghostID,
		SeedSelector: strings.TrimSpace(command.SeedSelector),
		Operation:    strings.TrimSpace(command.Operation),
		Args:         copyArgs(command.Args),
	})
	if err != nil {
		return session.Event{}, err
	}
	var out ghostExecuteEnvelopeResponse
	if err := c.call(ctx, ghostControlRequest{
		Action:       executeEnvelopeAction,
		CommandFrame: cmdFrame,
	}, &out); err != nil {
		return session.Event{}, err
	}
	fr, err := frame.ReadFrame(bytes.NewReader(out.EventFrame), frame.DefaultLimits())
	if err != nil {
		return session.Event{}, err
	}
	return session.DecodeEventFrame(fr)
}

// Status reads ghost identity/state from the ghost admin endpoint.
func (c *GhostControlClient) Status(ctx context.Context) (ghostLifecycleStatus, error) {
	var out ghostLifecycleStatus
	if err := c.call(ctx, ghostControlRequest{Action: statusAction}, &out); err != nil {
		return ghostLifecycleStatus{}, err
	}
	if strings.TrimSpace(out.GhostID) == "" {
		out.GhostID = strings.TrimSpace(out.GhostIDLegacy)
	}
	return out, nil
}

// GhostExecution is the Ghost-held record of one command execution, including its seed.result
// output (api_v1.toml [shapes.execution]).
type GhostExecution struct {
	CommandID    string
	ExecutionID  string
	IntentID     string
	GhostID      string
	SeedSelector string
	Operation    string
	Args         map[string]string
	Phase        string
	Outcome      string
	CompletedAt  time.Time
	SeedResult   GhostSeedResult
}

// GhostSeedResult is the seed.result part of a Ghost execution record.
type GhostSeedResult struct {
	SeedID   string
	Status   string
	Stdout   []byte
	Stderr   []byte
	ExitCode uint32
}

// Wire shape of Ghost's execution_by_command_id reply; Ghost encodes its execution state with
// untagged Go field names, which these field names match.
type ghostExecutionResponse struct {
	Found     bool           `json:"found"`
	Execution GhostExecution `json:"execution"`
}

// ExecutionByCommandID reads one execution record from the ghost admin endpoint
// (admin_security.toml [authorization.ghost_admin] execution_by_command_id). found is false when
// the Ghost holds no record for commandID.
func (c *GhostControlClient) ExecutionByCommandID(ctx context.Context, commandID string) (GhostExecution, bool, error) {
	var out ghostExecutionResponse
	req := ghostControlRequest{Action: executionByCommandIDAction, CommandID: strings.TrimSpace(commandID)}
	if err := c.call(ctx, req, &out); err != nil {
		return GhostExecution{}, false, err
	}
	return out.Execution, out.Found, nil
}

// GhostRecordedEvent is one terminal event in a Ghost's recent-events record. Ghost encodes it with
// untagged Go field names, which these field names match.
type GhostRecordedEvent struct {
	EventID     string
	CommandID   string
	IntentID    string
	GhostID     string
	SeedID      string
	Outcome     string
	TimestampMS uint64
}

// GhostVerificationRecord is one protocol correlation record from a Ghost's verification view
// (api_v1.toml [shapes.ghost_verification]); the JSON names are Ghost's own.
type GhostVerificationRecord struct {
	RequestID          string `json:"request_id"`
	TraceID            string `json:"trace_id"`
	CommandMessageID   uint64 `json:"command_message_id"`
	CommandMessageType uint32 `json:"command_message_type"`
	EventMessageType   uint32 `json:"event_message_type"`
	CommandID          string `json:"command_id"`
	ExecutionID        string `json:"execution_id"`
	EventID            string `json:"event_id"`
	GhostID            string `json:"ghost_id"`
	SeedID             string `json:"seed_id"`
	Operation          string `json:"operation"`
	Outcome            string `json:"outcome"`
	SeedStatus         string `json:"seed_status"`
	ExitCode           uint32 `json:"exit_code"`
	TimestampMS        uint64 `json:"timestamp_ms"`
	Status             string `json:"status"`
}

// RecentEvents reads a Ghost's recent terminal events (recent_events); limit <= 0 keeps the
// Ghost's default.
func (c *GhostControlClient) RecentEvents(ctx context.Context, limit int) ([]GhostRecordedEvent, error) {
	var out []GhostRecordedEvent
	if err := c.call(ctx, ghostControlRequest{Action: recentEventsAction, Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Verification reads a Ghost's protocol verification view (verification); limit <= 0 keeps the
// Ghost's default.
func (c *GhostControlClient) Verification(ctx context.Context, limit int) ([]GhostVerificationRecord, error) {
	var out []GhostVerificationRecord
	if err := c.call(ctx, ghostControlRequest{Action: verificationAction, Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ListSeeds reads available seed metadata from the ghost admin endpoint.
func (c *GhostControlClient) ListSeeds(ctx context.Context) ([]session.SeedInfo, error) {
	var out []ghostSeedMetadata
	if err := c.call(ctx, ghostControlRequest{Action: listSeedsAction}, &out); err != nil {
		return nil, err
	}
	seeds := make([]session.SeedInfo, 0, len(out))
	for i := range out {
		meta := out[i]
		seeds = append(seeds, session.SeedInfo{
			ID:          strings.TrimSpace(meta.ID),
			Name:        strings.TrimSpace(meta.Name),
			Description: strings.TrimSpace(meta.Description),
		})
	}
	return seeds, nil
}

// ListSeedCatalog reads full seed capability descriptors from the ghost admin endpoint.
func (c *GhostControlClient) ListSeedCatalog(ctx context.Context) ([]SeedCapability, error) {
	var out []ghostSeedCapability
	if err := c.call(ctx, ghostControlRequest{Action: listSeedCatalogAction}, &out); err != nil {
		return nil, err
	}
	caps := make([]SeedCapability, 0, len(out))
	for i := range out {
		item := out[i]
		caps = append(caps, SeedCapability{
			Metadata: seeds.SeedMetadata{
				ID:          strings.TrimSpace(item.Metadata.ID),
				Name:        strings.TrimSpace(item.Metadata.Name),
				Description: strings.TrimSpace(item.Metadata.Description),
			},
			Operations:     mapOperationSpecs(item.Operations),
			CommandCatalog: mapCommandTemplates(item.CommandCatalog),
		})
	}
	return caps, nil
}

// BindMirage marks a ghost admin endpoint as attached to one Mirage control plane.
// sessionPort is the mirage session listener port (e.g. "9000") so ghost can resolve the session address.
func (c *GhostControlClient) BindMirage(ctx context.Context, mirageID string, sessionPort string) error {
	return c.call(ctx, ghostControlRequest{
		Action:      bindMirageAction,
		MirageID:    strings.TrimSpace(mirageID),
		SessionPort: strings.TrimSpace(sessionPort),
	}, nil)
}

func mapOperationSpecs(in []ghostOperationSpec) []seeds.OperationSpec {
	if len(in) == 0 {
		return []seeds.OperationSpec{}
	}
	out := make([]seeds.OperationSpec, len(in))
	for i := range in {
		out[i] = seeds.OperationSpec{
			Name:        strings.TrimSpace(in[i].Name),
			Description: strings.TrimSpace(in[i].Description),
			Idempotent:  in[i].Idempotent,
		}
	}
	return out
}

func mapCommandTemplates(in []ghostCommandTemplate) []seeds.CommandTemplate {
	if len(in) == 0 {
		return []seeds.CommandTemplate{}
	}
	out := make([]seeds.CommandTemplate, len(in))
	for i := range in {
		out[i] = seeds.CommandTemplate{
			ID:              strings.TrimSpace(in[i].ID),
			Label:           strings.TrimSpace(in[i].Label),
			Description:     strings.TrimSpace(in[i].Description),
			SeedSelector:    strings.TrimSpace(in[i].SeedSelector),
			Operation:       strings.TrimSpace(in[i].Operation),
			Args:            mapCommandArgSpecs(in[i].Args),
			DefaultBlocking: in[i].DefaultBlocking,
		}
	}
	return out
}

func mapCommandArgSpecs(in []ghostCommandArgSpec) []seeds.CommandArgSpec {
	if len(in) == 0 {
		return []seeds.CommandArgSpec{}
	}
	out := make([]seeds.CommandArgSpec, len(in))
	for i := range in {
		out[i] = seeds.CommandArgSpec{
			Key:          strings.TrimSpace(in[i].Key),
			Prompt:       strings.TrimSpace(in[i].Prompt),
			Required:     in[i].Required,
			DefaultValue: strings.TrimSpace(in[i].DefaultValue),
			Multiline:    in[i].Multiline,
			Terminator:   strings.TrimSpace(in[i].Terminator),
		}
	}
	return out
}

func (c *GhostControlClient) call(ctx context.Context, req ghostControlRequest, out any) error {
	return c.callTimeout(ctx, c.timeout, req, out)
}

// One request/response exchange, over mTLS when configured, bounded by timeout.
func (c *GhostControlClient) callTimeout(ctx context.Context, timeout time.Duration, req ghostControlRequest, out any) error {
	addr := strings.TrimSpace(c.adminAddr)
	if addr == "" {
		return fmt.Errorf("mirage: ghost admin addr required")
	}
	if c.tlsErr != nil {
		return c.tlsErr
	}
	conn, err := adminauth.Dial(ctx, addr, c.tlsConfig, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()

	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(timeout))
	if _, err := conn.Write(line); err != nil {
		return err
	}

	_ = conn.SetReadDeadline(time.Now().Add(timeout))
	respLine, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp ghostControlResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return &GhostAdminError{Action: req.Action, Code: resp.ErrorCode, Message: strings.TrimSpace(resp.Error)}
	}
	if out == nil || len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

// GhostAdminSpawner provisions local ghosts through an existing root Ghost admin endpoint.
type GhostAdminSpawner struct {
	client *GhostControlClient
}

// NewGhostAdminSpawner constructs a spawner bound to one root ghost admin address.
func NewGhostAdminSpawner(client *GhostControlClient) *GhostAdminSpawner {
	return &GhostAdminSpawner{client: client}
}

// SpawnLocalGhost calls root ghost admin "spawn_ghost" for local provisioning.
func (s *GhostAdminSpawner) SpawnLocalGhost(ctx context.Context, req SpawnGhostRequest) (SpawnGhostResult, error) {
	return s.client.SpawnLocalGhost(ctx, req)
}

// GhostAdminCommandExecutor maps Mirage command dispatch to Ghost admin execute RPC.
type GhostAdminCommandExecutor struct {
	client *GhostControlClient
}

// NewGhostAdminCommandExecutor constructs a command executor backed by ghost admin RPC.
func NewGhostAdminCommandExecutor(client *GhostControlClient) *GhostAdminCommandExecutor {
	return &GhostAdminCommandExecutor{client: client}
}

// ExecuteCommand invokes one command on the local ghost admin boundary.
func (e *GhostAdminCommandExecutor) ExecuteCommand(ctx context.Context, cmd session.Command) (session.Event, error) {
	if e == nil || e.client == nil {
		return session.Event{}, fmt.Errorf("mirage: nil ghost executor client")
	}
	event, err := e.client.ExecuteAdminCommand(ctx, ghostAdminCommand{
		GhostID:      strings.TrimSpace(cmd.GhostID),
		CommandID:    strings.TrimSpace(cmd.CommandID),
		IntentID:     strings.TrimSpace(cmd.IntentID),
		SeedSelector: strings.TrimSpace(cmd.SeedSelector),
		Operation:    strings.TrimSpace(cmd.Operation),
		Args:         copyArgs(cmd.Args),
	})
	if err != nil {
		return session.Event{}, err
	}
	if event.CommandID == "" {
		event.CommandID = cmd.CommandID
	}
	if event.IntentID == "" {
		event.IntentID = cmd.IntentID
	}
	if event.GhostID == "" {
		event.GhostID = cmd.GhostID
	}
	if event.SeedID == "" {
		event.SeedID = cmd.SeedSelector
	}
	if event.TimestampMS == 0 {
		event.TimestampMS = uint64(time.Now().UnixMilli())
	}
	return event, event.Validate()
}

// GhostSeedBuildlogStore persists buildlog entries through a configured ghost seed.
type GhostSeedBuildlogStore struct {
	client       *GhostControlClient
	seedSelector string
	// commandSeq provides per-process uniqueness for buildlog command IDs.
	commandSeq atomic.Uint64
}

// NewGhostSeedBuildlogStore constructs a buildlog persistence sink using seed.kv/seed.fs.
func NewGhostSeedBuildlogStore(client *GhostControlClient, seedSelector string) *GhostSeedBuildlogStore {
	return &GhostSeedBuildlogStore{
		client:       client,
		seedSelector: strings.TrimSpace(seedSelector),
	}
}

// Persist writes one buildlog key/value entry through ghost admin execute.
func (s *GhostSeedBuildlogStore) Persist(ctx context.Context, key string, value string) error {
	if s == nil || s.client == nil {
		return fmt.Errorf("mirage: nil buildlog store")
	}
	selector := strings.TrimSpace(s.seedSelector)
	if selector == "" {
		selector = "seed.fs"
	}

	switch selector {
	case "seed.kv":
		_, err := s.client.ExecuteAdminCommand(ctx, ghostAdminCommand{
			CommandID:    s.nextBuildlogCommandID(),
			IntentID:     "intent.mirage.buildlog",
			SeedSelector: selector,
			Operation:    "put",
			Args: map[string]string{
				"key":   strings.TrimSpace(key),
				"value": value,
			},
		})
		return err
	case "seed.fs":
		path := strings.TrimSpace(key)
		if path == "" {
			return fmt.Errorf("mirage: buildlog key/path required")
		}
		_, err := s.client.ExecuteAdminCommand(ctx, ghostAdminCommand{
			CommandID:    s.nextBuildlogCommandID(),
			IntentID:     "intent.mirage.buildlog",
			SeedSelector: selector,
			Operation:    "write",
			Args: map[string]string{
				"path":    path,
				"content": value,
			},
		})
		return err
	default:
		return fmt.Errorf("mirage: unsupported buildlog seed selector %q", selector)
	}
}

// nextBuildlogCommandID returns a unique command_id for buildlog seed writes.
// Boundary: IDs satisfy the protocol required command_id field and are local-process unique.
func (s *GhostSeedBuildlogStore) nextBuildlogCommandID() string {
	seq := s.commandSeq.Add(1)
	return fmt.Sprintf("cmd.intent.mirage.buildlog.%d.%d", time.Now().UnixNano(), seq)
}
