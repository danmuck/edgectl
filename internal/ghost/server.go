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

type LifecyclePhase string

const (
	PhaseBoot      LifecyclePhase = "boot"
	PhaseAppeared  LifecyclePhase = "appeared"
	PhaseRadiating LifecyclePhase = "radiating"
	PhaseSeeded    LifecyclePhase = "seeded"
)

type GhostConfig struct {
	GhostID string
}

type SeedRegistry interface {
	Resolve(seedID string) (seeds.Seed, bool)
	ListMetadata() []seeds.SeedMetadata
}

type Lifecycle interface {
	Appear(cfg GhostConfig) error
	Radiate() error
	Seed(reg SeedRegistry) error
	Status() LifecycleStatus
}

type LifecycleStatus struct {
	GhostID   string
	Phase     LifecyclePhase
	SeedCount int
}

type Server struct {
	mu       sync.RWMutex
	ghostID  string
	phase    LifecyclePhase
	registry SeedRegistry
}

func NewServer() *Server {
	logs.Debug("ghost.NewServer")
	return &Server{phase: PhaseBoot}
}

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

func transitionError(current LifecyclePhase, expected LifecyclePhase) error {
	return fmt.Errorf("%w: have=%s want=%s", ErrLifecycleOrder, current, expected)
}
