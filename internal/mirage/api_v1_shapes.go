package mirage

import (
	"encoding/base64"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
)

// Response shapes for api/v1 (docs/architecture/definitions/api_v1.toml [shapes]). They exist so
// the HTTP boundary has fixed snake_case names; the admin endpoint's wire shapes are unchanged.

// apiV1Report is the report envelope (api_v1.toml [shapes.report]).
type apiV1Report struct {
	IntentID        string `json:"intent_id"`
	Phase           string `json:"phase"`
	Summary         string `json:"summary"`
	CompletionState string `json:"completion_state"`
	CommandID       string `json:"command_id,omitempty"`
	ExecutionID     string `json:"execution_id,omitempty"`
	EventID         string `json:"event_id,omitempty"`
	Outcome         string `json:"outcome,omitempty"`
	TimestampMS     uint64 `json:"timestamp_ms"`
}

// apiV1Status is the Mirage status read model (api_v1.toml [shapes.status]).
type apiV1Status struct {
	MirageID         string `json:"mirage_id"`
	Phase            string `json:"phase"`
	RegisteredGhosts int    `json:"registered_ghosts"`
	ActiveIntents    int    `json:"active_intents"`
	ReportCount      int    `json:"report_count"`
}

// apiV1IntentCommand is one planned command with its Mirage lifecycle state
// (api_v1.toml [shapes.intent_command]).
type apiV1IntentCommand struct {
	CommandID    string            `json:"command_id"`
	GhostID      string            `json:"ghost_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
	Blocking     bool              `json:"blocking"`
	State        string            `json:"state"`
	Attempts     int               `json:"attempts"`
	LastError    string            `json:"last_error"`
	UpdatedAtMS  int64             `json:"updated_at_ms"`
}

// apiV1IntentSnapshot is desired plus observed state for one intent
// (api_v1.toml [shapes.intent_snapshot]).
type apiV1IntentSnapshot struct {
	IntentID     string               `json:"intent_id"`
	Issue        AdminIssueRequest    `json:"issue"`
	ReceivedAtMS int64                `json:"received_at_ms"`
	Commands     []apiV1IntentCommand `json:"commands"`
	Reports      []apiV1Report        `json:"reports"`
	HasObserved  bool                 `json:"has_observed"`
	PendingCount int                  `json:"pending_count"`
}

// apiV1Ghost is one registered Ghost (api_v1.toml [shapes.ghost]).
type apiV1Ghost struct {
	GhostID        string             `json:"ghost_id"`
	RemoteAddr     string             `json:"remote_addr"`
	SeedList       []session.SeedInfo `json:"seed_list"`
	RegisteredAtMS int64              `json:"registered_at_ms"`
	LastEventAtMS  int64              `json:"last_event_at_ms"`
	EventCount     uint64             `json:"event_count"`
	Connected      bool               `json:"connected"`
}

// apiV1SeedMetadata, apiV1Operation, apiV1CommandArg, apiV1CommandTemplate and apiV1SeedCapability
// mirror one Ghost-published seed capability (api_v1.toml [shapes.seed_capability]).
type apiV1SeedMetadata struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type apiV1Operation struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Idempotent  bool   `json:"idempotent"`
}

type apiV1CommandArg struct {
	Key          string `json:"key"`
	Prompt       string `json:"prompt"`
	Required     bool   `json:"required"`
	DefaultValue string `json:"default_value"`
	Multiline    bool   `json:"multiline"`
	Terminator   string `json:"terminator"`
}

type apiV1CommandTemplate struct {
	ID              string            `json:"id"`
	Label           string            `json:"label"`
	Description     string            `json:"description"`
	SeedSelector    string            `json:"seed_selector"`
	Operation       string            `json:"operation"`
	DefaultBlocking bool              `json:"default_blocking"`
	Args            []apiV1CommandArg `json:"args"`
}

type apiV1SeedCapability struct {
	Metadata       apiV1SeedMetadata      `json:"metadata"`
	Operations     []apiV1Operation       `json:"operations"`
	CommandCatalog []apiV1CommandTemplate `json:"command_catalog"`
}

// apiV1GhostSeedCatalog is one Ghost's catalog and fetch status
// (api_v1.toml [shapes.ghost_seed_catalog]).
type apiV1GhostSeedCatalog struct {
	GhostID     string                `json:"ghost_id"`
	AdminAddr   string                `json:"admin_addr"`
	Connected   bool                  `json:"connected"`
	Error       string                `json:"error,omitempty"`
	SeedCatalog []apiV1SeedCapability `json:"seed_catalog"`
}

// apiV1SeedResult is seed.result output; stdout and stderr share one encoding
// (api_v1.toml [shapes.seed_result]).
type apiV1SeedResult struct {
	SeedID         string `json:"seed_id"`
	Status         string `json:"status"`
	ExitCode       uint32 `json:"exit_code"`
	Stdout         string `json:"stdout"`
	Stderr         string `json:"stderr"`
	OutputEncoding string `json:"output_encoding"`
}

// apiV1Execution is the Ghost execution record for one command (api_v1.toml [shapes.execution]).
type apiV1Execution struct {
	CommandID     string            `json:"command_id"`
	ExecutionID   string            `json:"execution_id"`
	IntentID      string            `json:"intent_id"`
	GhostID       string            `json:"ghost_id"`
	SeedSelector  string            `json:"seed_selector"`
	Operation     string            `json:"operation"`
	Args          map[string]string `json:"args"`
	Phase         string            `json:"phase"`
	Outcome       string            `json:"outcome"`
	CompletedAtMS int64             `json:"completed_at_ms"`
	SeedResult    apiV1SeedResult   `json:"seed_result"`
}

// Unix milliseconds, with the zero time as 0 (api_v1.toml [shapes]: zero means not set).
func apiV1Millis(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// Reports the dispatcher returned a type the endpoint does not expect; a programming error
// surfaced as an internal failure rather than a panic.
func apiV1BadData(action string, data any) error {
	return fmt.Errorf("mirage.api: %s returned unexpected %T", action, data)
}

func apiV1MapReport(r session.Report) apiV1Report {
	return apiV1Report{
		IntentID:        r.IntentID,
		Phase:           r.Phase,
		Summary:         r.Summary,
		CompletionState: r.CompletionState,
		CommandID:       r.CommandID,
		ExecutionID:     r.ExecutionID,
		EventID:         r.EventID,
		Outcome:         r.Outcome,
		TimestampMS:     r.TimestampMS,
	}
}

func apiV1MapReports(in []session.Report) []apiV1Report {
	out := make([]apiV1Report, 0, len(in))
	for i := range in {
		out = append(out, apiV1MapReport(in[i]))
	}
	return out
}

func apiV1Args(in map[string]string) map[string]string {
	if in == nil {
		return map[string]string{}
	}
	return in
}

func apiV1ShapeStatus(_ adminControlRequest, data any) (any, error) {
	st, ok := data.(LifecycleStatus)
	if !ok {
		return nil, apiV1BadData("status", data)
	}
	return apiV1Status{
		MirageID:         st.MirageID,
		Phase:            string(st.Phase),
		RegisteredGhosts: st.RegisteredGhosts,
		ActiveIntents:    st.ActiveIntents,
		ReportCount:      st.ReportCount,
	}, nil
}

// submit_issue returns no data; the reply names the intent that was accepted.
func apiV1ShapeIntentRef(req adminControlRequest, _ any) (any, error) {
	return map[string]string{"intent_id": req.Issue.IntentID}, nil
}

func apiV1ShapeIntentIDs(_ adminControlRequest, data any) (any, error) {
	ids, ok := data.([]string)
	if !ok {
		return nil, apiV1BadData("list_intents", data)
	}
	if ids == nil {
		ids = []string{}
	}
	return map[string][]string{"intent_ids": ids}, nil
}

// Flattens desired and observed state into the contract's read model; an unknown intent is a
// not-found error rather than found=false.
func apiV1ShapeSnapshot(req adminControlRequest, data any) (any, error) {
	resp, ok := data.(AdminSnapshotIntentResponse)
	if !ok {
		return nil, apiV1BadData("snapshot_intent", data)
	}
	if !resp.Found {
		return nil, fmt.Errorf("%w: intent_id %q", errAPIv1UnknownID, req.IntentID)
	}
	snap := resp.Snapshot
	issue := snap.Desired.Issue
	out := apiV1IntentSnapshot{
		IntentID: issue.IntentID,
		Issue: AdminIssueRequest{
			IntentID:         issue.IntentID,
			Actor:            issue.Actor,
			TargetScope:      issue.TargetScope,
			Objective:        issue.Objective,
			SeedDependencies: issue.SeedDependencies,
			Stages:           make([]AdminIssueStage, 0, len(issue.Stages)),
		},
		ReceivedAtMS: apiV1Millis(snap.Desired.ReceivedAt),
		Commands:     make([]apiV1IntentCommand, 0, len(snap.Desired.Commands)),
		Reports:      apiV1MapReports(snap.Observed.Reports),
		HasObserved:  snap.HasObserved,
		PendingCount: snap.PendingCount,
	}
	for i := range issue.Stages {
		stage := AdminIssueStage{
			ID:       issue.Stages[i].ID,
			Barrier:  issue.Stages[i].Barrier,
			Commands: make([]AdminIssueCommand, 0, len(issue.Stages[i].Commands)),
		}
		for _, c := range issue.Stages[i].Commands {
			stage.Commands = append(stage.Commands, AdminIssueCommand{
				GhostID:      c.GhostID,
				SeedSelector: c.SeedSelector,
				Operation:    c.Operation,
				Args:         apiV1Args(c.Args),
				Blocking:     c.Blocking,
			})
		}
		out.Issue.Stages = append(out.Issue.Stages, stage)
	}
	for i := range snap.Desired.Commands {
		planned := snap.Desired.Commands[i]
		cmd := apiV1IntentCommand{
			CommandID:    planned.Command.CommandID,
			GhostID:      planned.Command.GhostID,
			SeedSelector: planned.Command.SeedSelector,
			Operation:    planned.Command.Operation,
			Args:         apiV1Args(planned.Command.Args),
			Blocking:     planned.Blocking,
		}
		if st, ok := snap.Observed.CommandStates[cmd.CommandID]; ok {
			cmd.State = string(st.State)
			cmd.Attempts = st.Attempts
			cmd.LastError = st.LastError
			cmd.UpdatedAtMS = apiV1Millis(st.UpdatedAt)
		}
		out.Commands = append(out.Commands, cmd)
	}
	return out, nil
}

func apiV1ShapeReport(_ adminControlRequest, data any) (any, error) {
	r, ok := data.(session.Report)
	if !ok {
		return nil, apiV1BadData("reconcile_intent", data)
	}
	return apiV1MapReport(r), nil
}

func apiV1ShapeReconcileAll(_ adminControlRequest, data any) (any, error) {
	resp, ok := data.(AdminReconcileAllResponse)
	if !ok {
		return nil, apiV1BadData("reconcile_all", data)
	}
	return map[string][]apiV1Report{"reports": apiV1MapReports(resp.Reports)}, nil
}

func apiV1ShapeReports(_ adminControlRequest, data any) (any, error) {
	reports, ok := data.([]session.Report)
	if !ok {
		return nil, apiV1BadData("recent_reports", data)
	}
	return map[string][]apiV1Report{"reports": apiV1MapReports(reports)}, nil
}

func apiV1ShapeGhosts(_ adminControlRequest, data any) (any, error) {
	ghosts, ok := data.([]RegisteredGhost)
	if !ok {
		return nil, apiV1BadData("registered_ghosts", data)
	}
	out := make([]apiV1Ghost, 0, len(ghosts))
	for _, g := range ghosts {
		seedList := g.SeedList
		if seedList == nil {
			seedList = []session.SeedInfo{}
		}
		out = append(out, apiV1Ghost{
			GhostID:        g.GhostID,
			RemoteAddr:     g.RemoteAddr,
			SeedList:       seedList,
			RegisteredAtMS: apiV1Millis(g.RegisteredAt),
			LastEventAtMS:  apiV1Millis(g.LastEventAt),
			EventCount:     g.EventCount,
			Connected:      g.Connected,
		})
	}
	return map[string][]apiV1Ghost{"ghosts": out}, nil
}

func apiV1ShapeRoutes(_ adminControlRequest, data any) (any, error) {
	routes, ok := data.([]GhostRoute)
	if !ok {
		return nil, apiV1BadData("routing_table", data)
	}
	if routes == nil {
		routes = []GhostRoute{}
	}
	return map[string][]GhostRoute{"routes": routes}, nil
}

func apiV1ShapeServices(_ adminControlRequest, data any) (any, error) {
	services, ok := data.([]AvailableService)
	if !ok {
		return nil, apiV1BadData("available_services", data)
	}
	if services == nil {
		services = []AvailableService{}
	}
	return map[string][]AvailableService{"services": services}, nil
}

func apiV1ShapeSeedCatalogs(_ adminControlRequest, data any) (any, error) {
	catalogs, ok := data.([]GhostSeedCatalog)
	if !ok {
		return nil, apiV1BadData("seed_catalog", data)
	}
	out := make([]apiV1GhostSeedCatalog, 0, len(catalogs))
	for _, c := range catalogs {
		entry := apiV1GhostSeedCatalog{
			GhostID:     c.GhostID,
			AdminAddr:   c.AdminAddr,
			Connected:   c.Connected,
			Error:       c.Error,
			SeedCatalog: make([]apiV1SeedCapability, 0, len(c.SeedCatalog)),
		}
		for _, capability := range c.SeedCatalog {
			entry.SeedCatalog = append(entry.SeedCatalog, apiV1MapCapability(capability))
		}
		out = append(out, entry)
	}
	return map[string][]apiV1GhostSeedCatalog{"ghosts": out}, nil
}

func apiV1MapCapability(in SeedCapability) apiV1SeedCapability {
	out := apiV1SeedCapability{
		Metadata: apiV1SeedMetadata{
			ID:          in.Metadata.ID,
			Name:        in.Metadata.Name,
			Description: in.Metadata.Description,
		},
		Operations:     make([]apiV1Operation, 0, len(in.Operations)),
		CommandCatalog: make([]apiV1CommandTemplate, 0, len(in.CommandCatalog)),
	}
	for _, op := range in.Operations {
		out.Operations = append(out.Operations, apiV1Operation{
			Name:        op.Name,
			Description: op.Description,
			Idempotent:  op.Idempotent,
		})
	}
	for _, tpl := range in.CommandCatalog {
		out.CommandCatalog = append(out.CommandCatalog, apiV1MapTemplate(tpl))
	}
	return out
}

func apiV1MapTemplate(in seeds.CommandTemplate) apiV1CommandTemplate {
	out := apiV1CommandTemplate{
		ID:              in.ID,
		Label:           in.Label,
		Description:     in.Description,
		SeedSelector:    in.SeedSelector,
		Operation:       in.Operation,
		DefaultBlocking: in.DefaultBlocking,
		Args:            make([]apiV1CommandArg, 0, len(in.Args)),
	}
	for _, a := range in.Args {
		out.Args = append(out.Args, apiV1CommandArg{
			Key:          a.Key,
			Prompt:       a.Prompt,
			Required:     a.Required,
			DefaultValue: a.DefaultValue,
			Multiline:    a.Multiline,
			Terminator:   a.Terminator,
		})
	}
	return out
}

func apiV1ShapeRouteRef(_ adminControlRequest, data any) (any, error) {
	out, ok := data.(AdminAttachGhostResponse)
	if !ok {
		return nil, apiV1BadData("attach_ghost_admin", data)
	}
	return out, nil
}

func apiV1ShapeSpawn(_ adminControlRequest, data any) (any, error) {
	out, ok := data.(SpawnGhostResult)
	if !ok {
		return nil, apiV1BadData("spawn_local_ghost", data)
	}
	return out, nil
}

// Text output when both streams are valid UTF-8, otherwise base64 for both, so a client never
// has to guess per stream.
func apiV1ShapeExecution(e GhostExecution) apiV1Execution {
	result := apiV1SeedResult{
		SeedID:         e.SeedResult.SeedID,
		Status:         e.SeedResult.Status,
		ExitCode:       e.SeedResult.ExitCode,
		Stdout:         string(e.SeedResult.Stdout),
		Stderr:         string(e.SeedResult.Stderr),
		OutputEncoding: "utf8",
	}
	if !utf8.Valid(e.SeedResult.Stdout) || !utf8.Valid(e.SeedResult.Stderr) {
		result.Stdout = base64.StdEncoding.EncodeToString(e.SeedResult.Stdout)
		result.Stderr = base64.StdEncoding.EncodeToString(e.SeedResult.Stderr)
		result.OutputEncoding = "base64"
	}
	return apiV1Execution{
		CommandID:     e.CommandID,
		ExecutionID:   e.ExecutionID,
		IntentID:      e.IntentID,
		GhostID:       e.GhostID,
		SeedSelector:  e.SeedSelector,
		Operation:     e.Operation,
		Args:          apiV1Args(e.Args),
		Phase:         e.Phase,
		Outcome:       e.Outcome,
		CompletedAtMS: apiV1Millis(e.CompletedAt),
		SeedResult:    result,
	}
}
