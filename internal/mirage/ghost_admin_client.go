package mirage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

const (
	spawnGhostAction      = "spawn_ghost"
	executeEnvelopeAction = "execute_envelope"
	statusAction          = "status"
	listSeedsAction       = "list_seeds"
	bindMirageAction      = "bind_mirage"
)

type ghostControlRequest struct {
	Action       string            `json:"action"`
	Spawn        SpawnGhostRequest `json:"spawn,omitempty"`
	Command      ghostAdminCommand `json:"command,omitempty"`
	CommandFrame []byte            `json:"command_frame,omitempty"`
	MirageID     string            `json:"mirage_id,omitempty"`
}

type ghostAdminCommand struct {
	GhostID      string            `json:"ghost_id,omitempty"`
	CommandID    string            `json:"command_id,omitempty"`
	IntentID     string            `json:"intent_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
}

type ghostControlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
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
}

type ghostSeedMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// GhostControlClient is a TCP JSON control client for one root/local ghost admin endpoint.
type GhostControlClient struct {
	adminAddr string
	timeout   time.Duration
}

// NewGhostControlClient constructs a control client bound to one ghost admin address.
func NewGhostControlClient(adminAddr string) *GhostControlClient {
	return &GhostControlClient{
		adminAddr: strings.TrimSpace(adminAddr),
		timeout:   5 * time.Second,
	}
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

// BindMirage marks a ghost admin endpoint as attached to one Mirage control plane.
func (c *GhostControlClient) BindMirage(ctx context.Context, mirageID string) error {
	return c.call(ctx, ghostControlRequest{
		Action:   bindMirageAction,
		MirageID: strings.TrimSpace(mirageID),
	}, nil)
}

func (c *GhostControlClient) call(ctx context.Context, req ghostControlRequest, out any) error {
	addr := strings.TrimSpace(c.adminAddr)
	if addr == "" {
		return fmt.Errorf("mirage: ghost admin addr required")
	}
	dialer := net.Dialer{Timeout: c.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(c.timeout))
	if _, err := conn.Write(line); err != nil {
		return err
	}

	_ = conn.SetReadDeadline(time.Now().Add(c.timeout))
	respLine, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return err
	}
	var resp ghostControlResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return fmt.Errorf("mirage: ghost control %s failed: %s", req.Action, strings.TrimSpace(resp.Error))
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
func NewGhostAdminSpawner(adminAddr string) *GhostAdminSpawner {
	return &GhostAdminSpawner{
		client: NewGhostControlClient(adminAddr),
	}
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
