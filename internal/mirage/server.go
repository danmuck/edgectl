package mirage

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
)

var (
	ErrInvalidMirageID = errors.New("mirage: invalid mirage id")
	ErrLifecycleOrder  = errors.New("mirage: invalid lifecycle transition")
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
}

// Server owns Mirage lifecycle, orchestration boundary, and observed ghost registry.
type Server struct {
	mu sync.RWMutex

	mirageID string
	phase    LifecyclePhase

	registry map[string]*registeredGhostState

	loop *Orchestrator
}

// NewServer constructs Mirage server state in boot phase.
func NewServer() *Server {
	return &Server{
		phase:    PhaseBoot,
		registry: make(map[string]*registeredGhostState),
		loop:     NewOrchestrator(),
	}
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
	s.mu.RUnlock()

	snapshot := s.loop.Snapshot()
	return LifecycleStatus{
		MirageID:         id,
		Phase:            phase,
		RegisteredGhosts: ghosts,
		ActiveIntents:    snapshot.IntentCount,
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
		SeedList:   copySeedList(reg.SeedList),
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
	if ack, ok := state.ackByEvent[event.EventID]; ok {
		return ack
	}
	ack := session.EventAck{
		EventID:     event.EventID,
		CommandID:   event.CommandID,
		GhostID:     ghostID,
		AckStatus:   session.AckStatusAccepted,
		AckCode:     0,
		TimestampMS: uint64(time.Now().UnixMilli()),
	}
	state.ackByEvent[event.EventID] = ack
	state.meta.LastEventAt = time.Now()
	state.meta.EventCount++
	return ack
}

// RegisterExecutor binds command execution for one ghost_id in the orchestration boundary.
func (s *Server) RegisterExecutor(ghostID string, exec CommandExecutor) error {
	return s.loop.RegisterExecutor(ghostID, exec)
}

// SubmitIssue ingests desired state into Mirage orchestration.
func (s *Server) SubmitIssue(issue IssueEnv) error {
	return s.loop.SubmitIssue(issue)
}

// ReconcileIntent executes one orchestration pass for an intent.
func (s *Server) ReconcileIntent(ctx context.Context, intentID string) (session.Report, error) {
	return s.loop.ReconcileOnce(ctx, intentID)
}

// SnapshotIntent returns desired/observed state for one intent.
func (s *Server) SnapshotIntent(intentID string) (IntentSnapshot, bool) {
	return s.loop.SnapshotIntent(intentID)
}

func transitionError(from, to LifecyclePhase) error {
	return fmt.Errorf("%w: %s -> %s", ErrLifecycleOrder, from, to)
}
