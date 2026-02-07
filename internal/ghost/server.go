package ghost

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInvalidGhostID = errors.New("ghost: invalid ghost id")
	ErrLifecycleOrder = errors.New("ghost: invalid lifecycle transition")
	ErrSeedRegistry   = errors.New("ghost: invalid seed registry")
)

// LifecyclePhase describes Ghost runtime phase transitions.
type LifecyclePhase string

const (
	PhaseBoot      LifecyclePhase = "boot"
	PhaseAppeared  LifecyclePhase = "appeared"
	PhaseRadiating LifecyclePhase = "radiating"
	PhaseSeeded    LifecyclePhase = "seeded"
)

// GhostConfig configures identity at Ghost appear time.
type GhostConfig struct {
	GhostID string
}

// SeedRegistry resolves and lists seeds available to this Ghost instance.
type SeedRegistry interface {
	Resolve(seedID string) (seeds.Seed, bool)
	ListMetadata() []seeds.SeedMetadata
}

// Lifecycle defines Ghost server phase transitions.
type Lifecycle interface {
	Appear(cfg GhostConfig) error
	Radiate() error
	Seed(reg SeedRegistry) error
	Status() LifecycleStatus
}

// CommandBoundary defines command intake and execution-state query operations.
type CommandBoundary interface {
	HandleCommand(cmd CommandEnv) (ExecutionState, error)
	HandleCommandAndExecute(cmd CommandEnv) (EventEnv, error)
	GetExecution(executionID string) (ExecutionState, bool)
	GetByCommandID(commandID string) (ExecutionState, bool)
	ExecutionByCommandID(commandID string) (ExecutionState, bool)
	ExecutionByMessageID(messageID uint64) (ExecutionState, bool)
}

// LifecycleStatus reports current Ghost identity/phase/seed inventory size.
type LifecycleStatus struct {
	GhostID   string
	Phase     LifecyclePhase
	SeedCount int
}

// Server owns Ghost lifecycle, command intake, and execution state storage.
type Server struct {
	mu                 sync.RWMutex
	ghostID            string
	phase              LifecyclePhase
	registry           SeedRegistry
	executionByID      map[string]ExecutionState
	executionByCmdID   map[string]ExecutionState
	commandByMessageID map[uint64]string
}

// Ghost constructor for a server in boot phase with empty execution state.
func NewServer() *Server {
	logs.Debug("ghost.NewServer")
	return &Server{
		phase:              PhaseBoot,
		executionByID:      make(map[string]ExecutionState),
		executionByCmdID:   make(map[string]ExecutionState),
		commandByMessageID: make(map[uint64]string),
	}
}

// Ghost lifecycle transition: sets immutable identity and moves boot->appeared.
func (s *Server) Appear(cfg GhostConfig) error {
	logs.Debugf("ghost.Server.Appear ghost_id=%q", cfg.GhostID)
	id := strings.TrimSpace(cfg.GhostID)
	if id == "" {
		logs.Err("ghost.Server.Appear empty ghost_id")
		return ErrInvalidGhostID
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseBoot {
		logs.Errf("ghost.Server.Appear invalid transition phase=%s", s.phase)
		return transitionError(s.phase, PhaseAppeared)
	}

	s.ghostID = id
	s.phase = PhaseAppeared
	logs.Infof("ghost.Server.Appear ok ghost_id=%q phase=%s", s.ghostID, s.phase)
	return nil
}

// Ghost lifecycle transition: moves seeded->radiating to accept commands.
func (s *Server) Radiate() error {
	logs.Debug("ghost.Server.Radiate")
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseSeeded {
		logs.Errf("ghost.Server.Radiate invalid transition phase=%s", s.phase)
		return transitionError(s.phase, PhaseRadiating)
	}

	s.phase = PhaseRadiating
	logs.Infof("ghost.Server.Radiate ok ghost_id=%q phase=%s", s.ghostID, s.phase)
	return nil
}

// Ghost lifecycle transition: installs seed registry and moves appeared->seeded.
func (s *Server) Seed(reg SeedRegistry) error {
	logs.Debug("ghost.Server.Seed")
	if reg == nil {
		logs.Err("ghost.Server.Seed nil registry")
		return ErrSeedRegistry
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseAppeared {
		logs.Errf("ghost.Server.Seed invalid transition phase=%s", s.phase)
		return transitionError(s.phase, PhaseSeeded)
	}

	s.registry = reg
	s.phase = PhaseSeeded
	logs.Infof(
		"ghost.Server.Seed ok ghost_id=%q phase=%s seeds=%d",
		s.ghostID,
		s.phase,
		len(reg.ListMetadata()),
	)
	return nil
}

// Ghost status snapshot of identity, phase, and seed count.
func (s *Server) Status() LifecycleStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()

	count := 0
	if s.registry != nil {
		count = len(s.registry.ListMetadata())
	}

	status := LifecycleStatus{
		GhostID:   s.ghostID,
		Phase:     s.phase,
		SeedCount: count,
	}
	logs.Debugf(
		"ghost.Server.Status ghost_id=%q phase=%s seeds=%d",
		status.GhostID,
		status.Phase,
		status.SeedCount,
	)
	return status
}

// Ghost command boundary handler for validation, authorization, and recording.
func (s *Server) HandleCommand(cmd CommandEnv) (ExecutionState, error) {
	logs.Debugf(
		"ghost.Server.HandleCommand message_id=%d command_id=%q intent_id=%q ghost_id=%q",
		cmd.MessageID,
		cmd.CommandID,
		cmd.IntentID,
		cmd.GhostID,
	)
	if err := cmd.Validate(); err != nil {
		logs.Errf("ghost.Server.HandleCommand invalid command env err=%v", err)
		return ExecutionState{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.phase != PhaseRadiating {
		logs.Errf("ghost.Server.HandleCommand not radiating phase=%s", s.phase)
		return ExecutionState{}, ErrNotRadiating
	}

	if strings.TrimSpace(cmd.GhostID) != s.ghostID {
		logs.Errf(
			"ghost.Server.HandleCommand target mismatch target=%q local=%q",
			cmd.GhostID,
			s.ghostID,
		)
		return ExecutionState{}, ErrCommandTargetMismatch
	}

	if _, exists := s.executionByCmdID[strings.TrimSpace(cmd.CommandID)]; exists {
		logs.Errf("ghost.Server.HandleCommand duplicate command_id=%q", cmd.CommandID)
		return ExecutionState{}, ErrDuplicateCommandID
	}

	if _, exists := s.commandByMessageID[cmd.MessageID]; exists {
		logs.Errf("ghost.Server.HandleCommand duplicate message_id=%d", cmd.MessageID)
		return ExecutionState{}, ErrDuplicateMessageID
	}

	state := newExecutionState(cmd)
	s.executionByID[state.ExecutionID] = state
	s.executionByCmdID[state.CommandID] = state
	s.commandByMessageID[state.MessageID] = state.CommandID
	logs.Infof(
		"ghost.Server.HandleCommand accepted command_id=%q execution_id=%q message_id=%d",
		state.CommandID,
		state.ExecutionID,
		state.MessageID,
	)
	return state, nil
}

// Ghost execution-store update with terminal pipeline artifacts.
func (s *Server) completeExecution(
	executionID string,
	seedExec SeedExecuteEnv,
	seedResult SeedResultEnv,
	event EventEnv,
) {
	s.mu.Lock()
	defer s.mu.Unlock()

	state, ok := s.executionByID[executionID]
	if !ok {
		logs.Errf("ghost.Server.completeExecution missing execution_id=%q", executionID)
		return
	}

	state.SeedExecute = seedExec
	state.SeedResult = seedResult
	state.Event = event
	state.Outcome = event.Outcome
	state.Phase = ExecutionComplete

	s.executionByID[state.ExecutionID] = state
	s.executionByCmdID[state.CommandID] = state
	s.commandByMessageID[state.MessageID] = state.CommandID
	logs.Debugf(
		"ghost.Server.completeExecution execution_id=%q command_id=%q outcome=%q",
		state.ExecutionID,
		state.CommandID,
		state.Outcome,
	)
}

// Ghost execution lookup by execution_id.
func (s *Server) GetExecution(executionID string) (ExecutionState, bool) {
	key := strings.TrimSpace(executionID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.executionByID[key]
	logs.Debugf("ghost.Server.GetExecution execution_id=%q found=%v", key, ok)
	return state, ok
}

// Ghost execution lookup by command_id.
func (s *Server) GetByCommandID(commandID string) (ExecutionState, bool) {
	return s.ExecutionByCommandID(commandID)
}

// Ghost execution lookup by command_id.
func (s *Server) ExecutionByCommandID(commandID string) (ExecutionState, bool) {
	key := strings.TrimSpace(commandID)

	s.mu.RLock()
	defer s.mu.RUnlock()

	state, ok := s.executionByCmdID[key]
	logs.Debugf("ghost.Server.ExecutionByCommandID command_id=%q found=%v", key, ok)
	return state, ok
}

// Ghost execution lookup by message_id.
func (s *Server) ExecutionByMessageID(messageID uint64) (ExecutionState, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	commandID, ok := s.commandByMessageID[messageID]
	if !ok {
		logs.Debugf("ghost.Server.ExecutionByMessageID message_id=%d found=false", messageID)
		return ExecutionState{}, false
	}
	state, ok := s.executionByCmdID[commandID]
	logs.Debugf("ghost.Server.ExecutionByMessageID message_id=%d found=%v", messageID, ok)
	return state, ok
}

// Ghost snapshot of currently registered seed metadata.
func (s *Server) SeedMetadata() []seeds.SeedMetadata {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.registry == nil {
		return []seeds.SeedMetadata{}
	}
	list := s.registry.ListMetadata()
	out := make([]seeds.SeedMetadata, len(list))
	copy(out, list)
	return out
}

// Ghost lifecycle helper for stable transition mismatch errors.
func transitionError(current LifecyclePhase, expected LifecyclePhase) error {
	return fmt.Errorf("%w: have=%s want=%s", ErrLifecycleOrder, current, expected)
}
