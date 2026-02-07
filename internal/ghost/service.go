package ghost

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var (
	ErrInvalidHeartbeatInterval = errors.New("ghost: invalid heartbeat interval")
	ErrUnknownBuiltinSeed       = errors.New("ghost: unknown builtin seed")
)

type ServiceConfig struct {
	GhostID           string
	BuiltinSeedIDs    []string
	HeartbeatInterval time.Duration
}

func DefaultServiceConfig() ServiceConfig {
	return ServiceConfig{
		GhostID:           "ghost.local",
		BuiltinSeedIDs:    []string{"seed.flow"},
		HeartbeatInterval: 5 * time.Second,
	}
}

// Service runs the Ghost server lifecycle as a standalone process.
type Service struct {
	server *Server
	cfg    ServiceConfig
}

// NewService creates a Ghost service with default standalone config.
func NewService() *Service {
	return NewServiceWithConfig(DefaultServiceConfig())
}

// NewServiceWithConfig creates a Ghost service with explicit config.
func NewServiceWithConfig(cfg ServiceConfig) *Service {
	return &Service{
		server: NewServer(),
		cfg:    cfg,
	}
}

// Run starts Ghost lifecycle and blocks until process signal shutdown.
func (s *Service) Run() error {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := s.bootstrap(); err != nil {
		return err
	}
	return s.serve(ctx)
}

// Server returns the lifecycle/execution boundary owner for Ghost.
func (s *Service) Server() *Server {
	return s.server
}

func (s *Service) bootstrap() error {
	if s.cfg.HeartbeatInterval <= 0 {
		return ErrInvalidHeartbeatInterval
	}

	if err := s.server.Appear(GhostConfig{GhostID: s.cfg.GhostID}); err != nil {
		return err
	}

	reg, err := buildBuiltinRegistry(s.cfg.BuiltinSeedIDs)
	if err != nil {
		return err
	}
	if err := s.server.Seed(reg); err != nil {
		return err
	}
	if err := s.server.Radiate(); err != nil {
		return err
	}

	status := s.server.Status()
	logs.Infof(
		"ghost.Service.bootstrap ready ghost_id=%q phase=%s seeds=%d",
		status.GhostID,
		status.Phase,
		status.SeedCount,
	)
	return nil
}

func (s *Service) serve(ctx context.Context) error {
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			logs.Infof("ghost.Service.serve shutdown")
			return nil
		case <-ticker.C:
			status := s.server.Status()
			logs.Infof(
				"ghost.Service.heartbeat ghost_id=%q phase=%s seeds=%d",
				status.GhostID,
				status.Phase,
				status.SeedCount,
			)
		}
	}
}

func buildBuiltinRegistry(seedIDs []string) (*seeds.Registry, error) {
	reg := seeds.NewRegistry()

	seen := make(map[string]struct{})
	for _, raw := range seedIDs {
		id := strings.TrimSpace(raw)
		if id == "" || id == "none" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}

		switch id {
		case "seed.flow", "flow":
			if err := reg.Register(seeds.NewFlowSeed()); err != nil {
				return nil, err
			}
		case "seed.mongod", "mongod":
			if err := reg.Register(seeds.NewMongodSeed()); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("%w: %s", ErrUnknownBuiltinSeed, id)
		}
	}

	return reg, nil
}
