package mirage

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"sync"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/protocol/session"
)

var (
	ErrInvalidMirageID = errors.New("mirage: invalid mirage id")
	ErrLifecycleOrder  = errors.New("mirage: invalid lifecycle transition")
	ErrNoGhostSpawner  = errors.New("mirage: no ghost spawner configured")
)

// LifecyclePhase describes Mirage runtime phase transitions.
type LifecyclePhase string

const (
	PhaseBoot     LifecyclePhase = "boot"
	PhaseAppeared LifecyclePhase = "appeared"
	PhaseShimmer  LifecyclePhase = "shimmer"
	PhaseSeeded   LifecyclePhase = "seeded"
)

// MirageConfig configures identity at Mirage appear time.
type MirageConfig struct {
	MirageID string
}

// LifecycleStatus reports current Mirage identity and registration/orchestration shape.
type LifecycleStatus struct {
	MirageID         string
	Phase            LifecyclePhase
	RegisteredGhosts int
	ActiveIntents    int
	ReportCount      int
}

// SpawnGhostRequest defines one local Ghost provisioning request from Mirage. Field names on the
// wire match Ghost's spawn_ghost request (ghost.SpawnGhostRequest), which Mirage forwards it to.
type SpawnGhostRequest struct {
	TargetName string `json:"target_name"`
	AdminAddr  string `json:"admin_addr"`
}

// SpawnGhostResult describes one provisioned local Ghost endpoint; wire names match Ghost's reply.
type SpawnGhostResult struct {
	TargetName string `json:"target_name"`
	GhostID    string `json:"ghost_id"`
	AdminAddr  string `json:"admin_addr"`
}

// GhostSpawner provisions local Ghost nodes via a boundary adapter.
type GhostSpawner interface {
	SpawnLocalGhost(ctx context.Context, req SpawnGhostRequest) (SpawnGhostResult, error)
}

// Server owns Mirage lifecycle, orchestration boundary, and observed ghost registry.
type Server struct {
	mu sync.RWMutex

	mirageID string
	phase    LifecyclePhase

	registry map[string]*registeredGhostState

	loop *Orchestrator

	reports []session.Report
	spawner GhostSpawner

	// Dependency install proposals, approvals, and audit trail (Slice F).
	deps *DependencyLedger

	// Clock for ack timestamps and duplicate-window expiry; replaceable in tests.
	now func() time.Time
}

// Minimum retention for accepted event_id acks (reliability.toml idempotency.duplicate_window).
const DuplicateWindow = 15 * time.Minute

// NewServer constructs Mirage server state in boot phase.
func NewServer() *Server {
	return &Server{
		phase:    PhaseBoot,
		registry: make(map[string]*registeredGhostState),
		loop:     NewOrchestrator(),
		reports:  make([]session.Report, 0),
		now:      time.Now,
		deps:     NewDependencyLedger(),
	}
}

// Dependencies returns the dependency proposal ledger.
func (s *Server) Dependencies() *DependencyLedger {
	return s.deps
}

// Appear sets immutable Mirage identity and transitions boot->appeared.
func (s *Server) Appear(cfg MirageConfig) error {
	id := strings.TrimSpace(cfg.MirageID)
	if id == "" {
		return ErrInvalidMirageID
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseBoot {
		return transitionError(s.phase, PhaseAppeared)
	}
	s.mirageID = id
	s.phase = PhaseAppeared
	return nil
}

// Shimmer transitions appeared->shimmer to represent orchestration boundary ownership.
func (s *Server) Shimmer() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseAppeared {
		return transitionError(s.phase, PhaseShimmer)
	}
	s.phase = PhaseShimmer
	return nil
}

// Seed transitions shimmer->seeded once runtime connectors are wired.
func (s *Server) Seed() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.phase != PhaseShimmer {
		return transitionError(s.phase, PhaseSeeded)
	}
	s.phase = PhaseSeeded
	return nil
}

// Status returns current Mirage lifecycle and boundary state.
func (s *Server) Status() LifecycleStatus {
	s.mu.RLock()
	id := s.mirageID
	phase := s.phase
	ghosts := len(s.registry)
	reports := len(s.reports)
	s.mu.RUnlock()

	snapshot := s.loop.Snapshot()
	return LifecycleStatus{
		MirageID:         id,
		Phase:            phase,
		RegisteredGhosts: ghosts,
		ActiveIntents:    snapshot.IntentCount,
		ReportCount:      reports,
	}
}

// SnapshotRegisteredGhosts returns observed registration state for all ghosts.
func (s *Server) SnapshotRegisteredGhosts() []RegisteredGhost {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]RegisteredGhost, 0, len(s.registry))
	for _, state := range s.registry {
		meta := state.meta
		meta.SeedList = copySeedList(meta.SeedList)
		out = append(out, meta)
	}
	return out
}

// UpsertRegistration records registration metadata and returns accepted ack payload.
func (s *Server) UpsertRegistration(remoteAddr string, reg session.Registration) session.RegistrationAck {
	now := uint64(time.Now().UnixMilli())
	registered := RegisteredGhost{
		GhostID:    reg.GhostID,
		RemoteAddr: remoteAddr,
		SeedList:   enrichSeedListForGhost(reg.GhostID, remoteAddr, reg.SeedList),
		Connected:  true,
	}

	s.mu.Lock()
	state, ok := s.registry[reg.GhostID]
	if !ok {
		state = &registeredGhostState{ackByEvent: make(map[string]session.EventAck)}
		s.registry[reg.GhostID] = state
	}
	if state.meta.RegisteredAt.IsZero() {
		state.meta.RegisteredAt = time.Now()
	}
	registered.RegisteredAt = state.meta.RegisteredAt
	registered.LastEventAt = state.meta.LastEventAt
	registered.EventCount = state.meta.EventCount
	state.meta = registered
	s.mu.Unlock()

	return session.RegistrationAck{
		Status:      session.AckStatusAccepted,
		Code:        0,
		Message:     "registered",
		GhostID:     reg.GhostID,
		TimestampMS: now,
	}
}

// MarkGhostDisconnected marks the connection state while preserving observed counters.
func (s *Server) MarkGhostDisconnected(ghostID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.registry[ghostID]
	if !ok {
		return
	}
	state.meta.Connected = false
	state.meta.RemoteAddr = ""
}

// AcceptEvent ingests one event and returns deterministic idempotent event.ack.
func (s *Server) AcceptEvent(ghostID string, event session.Event) session.EventAck {
	s.mu.Lock()
	defer s.mu.Unlock()
	state, ok := s.registry[ghostID]
	if !ok {
		state = &registeredGhostState{
			meta: RegisteredGhost{
				GhostID:      ghostID,
				RegisteredAt: time.Now(),
			},
			ackByEvent: make(map[string]session.EventAck),
		}
		s.registry[ghostID] = state
	}
	s.pruneExpiredAcksLocked(state)
	if ack, ok := state.ackByEvent[event.EventID]; ok {
		return ack
	}
	now := s.now()
	ack := session.EventAck{
		EventID:     event.EventID,
		CommandID:   event.CommandID,
		GhostID:     ghostID,
		AckStatus:   session.AckStatusAccepted,
		AckCode:     0,
		TimestampMS: uint64(now.UnixMilli()),
	}
	state.ackByEvent[event.EventID] = ack
	state.meta.LastEventAt = now
	state.meta.EventCount++
	return ack
}

// RegisterExecutor binds command execution for one ghost_id in the orchestration boundary.
func (s *Server) RegisterExecutor(ghostID string, exec CommandExecutor) error {
	return s.loop.RegisterExecutor(ghostID, exec)
}

// SetCommandRetryPolicy configures command dispatch retry bounds in the orchestration boundary.
func (s *Server) SetCommandRetryPolicy(p CommandRetryPolicy) {
	s.loop.SetCommandRetryPolicy(p)
}

// SubmitIssue ingests desired state into Mirage orchestration.
func (s *Server) SubmitIssue(issue IssueEnv) error {
	return s.loop.SubmitIssue(issue)
}

// ReconcileIntent executes one orchestration pass for an intent.
func (s *Server) ReconcileIntent(ctx context.Context, intentID string) (session.Report, error) {
	report, err := s.loop.ReconcileOnce(ctx, intentID)
	if err != nil {
		return session.Report{}, err
	}
	s.appendReport(report)
	return report, nil
}

// SnapshotIntent returns desired/observed state for one intent.
func (s *Server) SnapshotIntent(intentID string) (IntentSnapshot, bool) {
	return s.loop.SnapshotIntent(intentID)
}

// ListIntentIDs returns sorted desired intent ids.
func (s *Server) ListIntentIDs() []string {
	return s.loop.ListIntentIDs()
}

// ObserveEvent ingests a ghost event into orchestration observed state when command is known.
// The command must be planned for boundGhostID (ErrCommandGhostMismatch otherwise, no mutation)
// and already dispatched by Mirage (ErrCommandNotDispatched otherwise, no mutation).
// Duplicate observations return the latest report without appending to report history.
func (s *Server) ObserveEvent(boundGhostID string, event session.Event) (session.Report, bool, bool, error) {
	return s.observeEvent(boundGhostID, event, 0)
}

// observeEvent is ObserveEvent with the inbound frame message_id for custody logging.
func (s *Server) observeEvent(boundGhostID string, event session.Event, messageID uint64) (session.Report, bool, bool, error) {
	report, matched, duplicate, err := s.loop.ingestObservedEvent(boundGhostID, event, messageID)
	if err != nil || !matched || duplicate {
		return report, matched, duplicate, err
	}
	s.appendReport(report)
	return report, matched, false, nil
}

// EventIngest is the outcome of one session event delivery at the Mirage boundary.
type EventIngest struct {
	Ack session.EventAck
	// Rejected is true when the event does not match the session's bound ghost_id or names a
	// planned command that was never dispatched;
	// nothing was observed or recorded and the session must be closed.
	Rejected     bool
	RejectReason string
	// Duplicate is true when event_id was already accepted; nothing was re-observed.
	Duplicate bool
	Report    session.Report
	Matched   bool
	// ObserveErr reports an observe failure; the event is still acked per reliability.toml.
	ObserveErr error
}

// IngestEvent applies identity binding, event_id dedupe, and observation, and returns the ack.
// Binding (transport_security.toml identity_binding; errors.toml declared_peer_mismatch=1000):
// the declared ghost_id is checked first; the command's planned ghost is checked atomically with
// observation. An event for a planned command Mirage never dispatched is not feedback for a
// delegated command (protocol.toml envelope flow) and is rejected the same way, so a session peer
// cannot complete work that was never sent. Each is rejected with no observation and no stored ack. A previously
// accepted event_id short-circuits to its stored ack with no state mutation; stored acks only
// exist for events that already passed binding for this same session ghost.
func (s *Server) IngestEvent(ghostID string, event session.Event) EventIngest {
	return s.ingestEvent(ghostID, event, 0)
}

// ingestEvent is IngestEvent with the session frame message_id; the orchestrator logs a matched
// event (and its report) in causal order, the session loop logs rejected/duplicate/error cases.
func (s *Server) ingestEvent(ghostID string, event session.Event, messageID uint64) EventIngest {
	bound := strings.TrimSpace(ghostID)
	if declared := strings.TrimSpace(event.GhostID); declared != bound {
		return s.rejectedIngest(bound, event,
			fmt.Sprintf("event ghost_id=%q does not match session ghost_id=%q", declared, bound))
	}
	if ack, ok := s.acceptedAck(bound, event.EventID); ok {
		return EventIngest{Ack: ack, Duplicate: true}
	}
	report, matched, duplicate, err := s.observeEvent(bound, event, messageID)
	if errors.Is(err, ErrCommandGhostMismatch) || errors.Is(err, ErrCommandNotDispatched) {
		return s.rejectedIngest(bound, event, err.Error())
	}
	return EventIngest{
		Ack:        s.AcceptEvent(bound, event),
		Duplicate:  duplicate,
		Report:     report,
		Matched:    matched,
		ObserveErr: err,
	}
}

// Rejected ingest result: ack_status=rejected, ack_code=1000, nothing stored.
func (s *Server) rejectedIngest(ghostID string, event session.Event, reason string) EventIngest {
	return EventIngest{
		Ack: session.EventAck{
			EventID:     event.EventID,
			CommandID:   event.CommandID,
			GhostID:     ghostID,
			AckStatus:   session.AckStatusRejected,
			AckCode:     schema.ErrCodeTransportFailure,
			TimestampMS: uint64(s.now().UnixMilli()),
		},
		Rejected:     true,
		RejectReason: reason,
	}
}

// Stored ack for an event_id accepted inside the duplicate window.
func (s *Server) acceptedAck(ghostID string, eventID string) (session.EventAck, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.registry[ghostID]
	if !ok {
		return session.EventAck{}, false
	}
	ack, ok := state.ackByEvent[eventID]
	if !ok || s.ackExpiredLocked(ack) {
		return session.EventAck{}, false
	}
	return ack, true
}

// Acks older than DuplicateWindow are eligible for pruning.
func (s *Server) ackExpiredLocked(ack session.EventAck) bool {
	acceptedAt := time.UnixMilli(int64(ack.TimestampMS))
	return s.now().Sub(acceptedAt) >= DuplicateWindow
}

// Drops expired acks for one ghost. Caller holds s.mu for write.
func (s *Server) pruneExpiredAcksLocked(state *registeredGhostState) {
	for eventID, ack := range state.ackByEvent {
		if s.ackExpiredLocked(ack) {
			delete(state.ackByEvent, eventID)
		}
	}
}

// RecentReports returns bounded report history for user-boundary emission.
func (s *Server) RecentReports(limit int) []session.Report {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if limit <= 0 || len(s.reports) <= limit {
		out := make([]session.Report, len(s.reports))
		copy(out, s.reports)
		return out
	}
	out := make([]session.Report, limit)
	copy(out, s.reports[len(s.reports)-limit:])
	return out
}

// SetGhostSpawner binds a local-ghost provisioning adapter.
func (s *Server) SetGhostSpawner(spawner GhostSpawner) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.spawner = spawner
}

// SpawnLocalGhost provisions one local ghost endpoint through configured spawner.
func (s *Server) SpawnLocalGhost(ctx context.Context, req SpawnGhostRequest) (SpawnGhostResult, error) {
	s.mu.RLock()
	spawner := s.spawner
	s.mu.RUnlock()
	if spawner == nil {
		return SpawnGhostResult{}, ErrNoGhostSpawner
	}
	return spawner.SpawnLocalGhost(ctx, req)
}

func (s *Server) appendReport(report session.Report) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reports = append(s.reports, report)
}

func transitionError(from, to LifecyclePhase) error {
	return fmt.Errorf("%w: %s -> %s", ErrLifecycleOrder, from, to)
}

// enrichSeedListForGhost adds runtime-discovery details to registration seed metadata.
func enrichSeedListForGhost(ghostID string, remoteAddr string, in []session.SeedInfo) []session.SeedInfo {
	out := copySeedList(in)
	host := hostFromRemoteAddr(remoteAddr)
	for i := range out {
		seedID := strings.TrimSpace(out[i].ID)
		desc := strings.TrimSpace(out[i].Description)
		switch seedID {
		case "seed.mongod":
			uri := fmt.Sprintf("mongodb://%s/?directConnection=true", net.JoinHostPort(host, "27017"))
			desc = appendDiscoveryField(desc, "endpoint_uri=", uri)
			desc = appendDiscoveryField(desc, "host=", host)
			desc = appendDiscoveryField(desc, "seed_scope=", "network_service")
		case "seed.fs":
			desc = appendDiscoveryField(desc, "path_root=", fsSeedRootForGhost(ghostID))
			desc = appendDiscoveryField(desc, "seed_scope=", "ghost_local_filesystem")
		case "seed.kv":
			desc = appendDiscoveryField(desc, "persistence=", "in_memory")
			desc = appendDiscoveryField(desc, "seed_scope=", "ghost_local_cache")
		case "seed.flow":
			desc = appendDiscoveryField(desc, "seed_scope=", "control_plane")
			desc = appendDiscoveryField(desc, "dispatch=", "ghost_execute")
		case "seed.docker":
			desc = appendDiscoveryField(desc, "host=", host)
			desc = appendDiscoveryField(desc, "seed_scope=", "host_container_runtime")
		case "seed.host":
			desc = appendDiscoveryField(desc, "seed_scope=", "host_introspection")
		default:
			desc = appendDiscoveryField(desc, "seed_scope=", "ghost_local")
		}
		desc = appendDiscoveryField(desc, "ghost_id=", strings.TrimSpace(ghostID))
		out[i].Description = desc
	}
	return out
}

func fsSeedRootForGhost(ghostID string) string {
	id := strings.TrimSpace(ghostID)
	if id == "" {
		id = "ghost.local"
	}
	return "local/dir/" + id
}

func appendDiscoveryField(desc string, key string, value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return strings.TrimSpace(desc)
	}
	token := key + v
	base := strings.TrimSpace(desc)
	if strings.Contains(base, token) {
		return base
	}
	if base == "" {
		return token
	}
	return base + "; " + token
}

func hostFromRemoteAddr(remoteAddr string) string {
	trimmed := strings.TrimSpace(remoteAddr)
	if trimmed == "" {
		return "127.0.0.1"
	}
	host, _, err := net.SplitHostPort(trimmed)
	if err != nil {
		return trimmed
	}
	host = strings.TrimSpace(host)
	if host == "" {
		return "127.0.0.1"
	}
	return host
}
