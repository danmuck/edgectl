package main

import (
	"bufio"
	"encoding/json"
	"net"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/mirage"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

// ghostConfigFile persists Ghost targets configured for the client.
type ghostConfigFile struct {
	ClearScreenAfterCommand bool                `toml:"clear_screen_after_command"`
	Targets                 []ghostTargetConfig `toml:"targets"`
}

// ghostTargetConfig binds a display name to a Ghost admin endpoint.
type ghostTargetConfig struct {
	Name    string `toml:"name"`
	Addr    string `toml:"addr"`
	GhostID string `toml:"ghost_id"`
}

// mirageConfigFile persists one Mirage control-plane target plus local Ghost linkage.
type mirageConfigFile struct {
	Targets             []mirageTargetConfig `toml:"targets"`
	LocalGhostID        string               `toml:"local_ghost_id"`
	LocalGhostAdminAddr string               `toml:"local_ghost_admin_addr"`
}

type mirageTargetConfig struct {
	Name     string `toml:"name"`
	Addr     string `toml:"addr"`
	MirageID string `toml:"mirage_id"`
}

// GhostAdminCommand is the input envelope for admin-triggered Ghost command execution.
type GhostAdminCommand struct {
	IntentID     string            `json:"intent_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
}

// controlRequest is one line-delimited control request payload.
type controlRequest struct {
	Action       string                  `json:"action"`
	Limit        int                     `json:"limit,omitempty"`
	CommandID    string                  `json:"command_id,omitempty"`
	CommandFrame []byte                  `json:"command_frame,omitempty"`
	Spawn        ghost.SpawnGhostRequest `json:"spawn,omitempty"`
}

// controlResponse is one line-delimited control response payload.
type controlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// MirageIssueCommand defines one command step for mirage issue submission.
type MirageIssueCommand struct {
	GhostID      string            `json:"ghost_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
	Blocking     bool              `json:"blocking"`
}

// Defines a stage of a single issue's schedule
type MirageIssueStage struct {
	ID       string               `json:"id"`
	Commands []MirageIssueCommand `json:"commands"`
	Barrier  bool                 `json:"barrier"`
}

// Defines one issue ingress payload for mirage admin control.
type MirageIssueRequest struct {
	IntentID         string             `json:"intent_id"`
	Actor            string             `json:"actor"`
	TargetScope      string             `json:"target_scope"`
	Objective        string             `json:"objective"`
	SeedDependencies []string           `json:"seed_dependencies,omitempty"`
	Stages           []MirageIssueStage `json:"stages"`
}

// Defines one guided argument prompt for a catalog command template.
type CommandArgSpec struct {
	Key          string
	Prompt       string
	Required     bool
	DefaultValue string
	Multiline    bool
	Terminator   string
}

// Defines one predeclared command shape used by Ghost/Mirage wizards.
type CommandTemplate struct {
	ID              string
	Label           string
	Description     string
	SeedSelector    string
	Operation       string
	Args            []CommandArgSpec
	DefaultBlocking bool
}

// Orchestration context for multi-stage intent.
type MirageIntentContext struct {
	IntentID string
	Actor    string
	Args     map[string]string
	GhostID  string

	// Discovered at runtime
	Services []MirageAvailableService
	Routes   []MirageRoute
}

// Binds one intent wizard entry to an orchestrator that produces stages.
// Templates requiring user ghost selection set RequiresGhostSelection.
type MirageIntentTemplate struct {
	ID                     string
	Label                  string
	Description            string
	Args                   []CommandArgSpec
	SeedDependencies       []string
	RequiresGhostSelection bool
	Orchestrator           func(ctx MirageIntentContext) ([]MirageIssueStage, error)
}

type MirageAttachGhostResponse struct {
	GhostID   string `json:"ghost_id"`
	AdminAddr string `json:"admin_addr"`
}

type MirageRoute struct {
	GhostID   string `json:"ghost_id"`
	AdminAddr string `json:"admin_addr"`
	Connected bool   `json:"connected"`
}

type MirageAvailableService struct {
	SeedID   string   `json:"seed_id"`
	GhostIDs []string `json:"ghost_ids"`
}

type mirageControlRequest struct {
	Action         string                    `json:"action"`
	Limit          int                       `json:"limit,omitempty"`
	IntentID       string                    `json:"intent_id,omitempty"`
	Issue          MirageIssueRequest        `json:"issue,omitempty"`
	Spawn          mirage.SpawnGhostRequest  `json:"spawn,omitempty"`
	Deploy         mirage.DeployGhostRequest `json:"deploy,omitempty"`
	GhostAdminAddr string                    `json:"ghost_admin_addr,omitempty"`
}

type mirageControlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

type mirageSnapshotIntentResponse struct {
	Found    bool                  `json:"found"`
	Snapshot mirage.IntentSnapshot `json:"snapshot"`
}

type mirageReconcileAllResponse struct {
	Reports []session.Report `json:"reports"`
}

// Holds execute_envelope action output.
type executeEnvelopeResponse struct {
	EventFrame []byte `json:"event_frame"`
}

// Holds execution lookup output.
type executionLookupResponse struct {
	Found     bool                 `json:"found"`
	Execution ghost.ExecutionState `json:"execution"`
}

// RemoteGhostAdmin is a TCP client for ghostctl admin control endpoint.
type RemoteGhostAdmin struct {
	addr string
	conn net.Conn
	r    *bufio.Reader
}

// GhostTarget maps a friendly name to a concrete Ghost admin implementation.
type GhostTarget struct {
	Name  string
	Admin GhostAdmin
}

// RemoteMirageAdmin is a TCP client for miragectl admin control endpoint.
type RemoteMirageAdmin struct {
	addr string
	conn net.Conn
	r    *bufio.Reader
}

// MirageTarget maps a friendly name to a concrete Mirage admin implementation.
type MirageTarget struct {
	Name  string
	Admin MirageAdmin
}

// App hosts interactive state and persisted target references.
type App struct {
	reader        *bufio.Reader
	ghostCfgPath  string
	mirageCfgPath string
	ghostCfg      ghostConfigFile
	mirageCfg     mirageConfigFile
	targets       []GhostTarget
	activeTarget  int
	mirageTargets []MirageTarget
	activeMirage  int
	clearScreen   bool
	launchMode    string
}
