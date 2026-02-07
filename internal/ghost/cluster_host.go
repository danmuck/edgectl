package ghost

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	logs "github.com/danmuck/smplog"
)

var (
	ErrClusterHostDisabled = errors.New("ghost: cluster host disabled")
	ErrManagedGhostExists  = errors.New("ghost: managed ghost already exists")
)

// SpawnGhostRequest defines one child Ghost provisioning request.
type SpawnGhostRequest struct {
	TargetName string `json:"target_name"`
	AdminAddr  string `json:"admin_addr"`
}

// SpawnGhostResult describes one provisioned child Ghost target.
type SpawnGhostResult struct {
	TargetName string `json:"target_name"`
	GhostID    string `json:"ghost_id"`
	AdminAddr  string `json:"admin_addr"`
}

type managedGhost struct {
	name   string
	cfg    ServiceConfig
	cancel context.CancelFunc
	done   chan error
}

type clusterHost struct {
	mu      sync.Mutex
	managed map[string]*managedGhost
}

func newClusterHost() clusterHost {
	return clusterHost{
		managed: make(map[string]*managedGhost),
	}
}

func (c *clusterHost) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.managed)
}

// SpawnManagedGhost starts a child Ghost service from the current host Ghost.
func (s *Service) SpawnManagedGhost(req SpawnGhostRequest) (SpawnGhostResult, error) {
	if !s.cfg.EnableClusterHost {
		logs.Warnf("ghost.cluster spawn rejected reason=cluster_host_disabled target=%q addr=%q", req.TargetName, req.AdminAddr)
		return SpawnGhostResult{}, ErrClusterHostDisabled
	}
	targetSuffix := normalizeNodeSuffix(req.TargetName)
	adminAddr := strings.TrimSpace(req.AdminAddr)
	if targetSuffix == "" {
		return SpawnGhostResult{}, fmt.Errorf("ghost: target_name required")
	}
	if adminAddr == "" {
		return SpawnGhostResult{}, fmt.Errorf("ghost: admin_addr required")
	}

	host := s.server.Status()
	targetName := host.GhostID + "." + targetSuffix
	ghostID := host.GhostID + "." + targetSuffix

	s.cluster.mu.Lock()
	if _, exists := s.cluster.managed[targetName]; exists {
		s.cluster.mu.Unlock()
		logs.Warnf("ghost.cluster spawn rejected reason=exists target=%q addr=%q", targetName, adminAddr)
		return SpawnGhostResult{}, fmt.Errorf("%w: %s", ErrManagedGhostExists, targetName)
	}
	s.cluster.mu.Unlock()

	cfg := DefaultServiceConfig()
	cfg.GhostID = ghostID
	cfg.BuiltinSeedIDs = append([]string{}, s.cfg.BuiltinSeedIDs...)
	cfg.HeartbeatInterval = s.cfg.HeartbeatInterval
	cfg.AdminListenAddr = adminAddr
	cfg.EnableClusterHost = false
	cfg.Mirage.Policy = MiragePolicyHeadless

	child := NewServiceWithConfig(cfg)
	if err := child.bootstrap(); err != nil {
		return SpawnGhostResult{}, err
	}
	runCtx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- child.serve(runCtx)
	}()

	s.cluster.mu.Lock()
	s.cluster.managed[targetName] = &managedGhost{
		name:   targetName,
		cfg:    cfg,
		cancel: cancel,
		done:   done,
	}
	s.cluster.mu.Unlock()
	logs.Infof("ghost.cluster child created target=%q ghost_id=%q addr=%q", targetName, ghostID, adminAddr)

	return SpawnGhostResult{
		TargetName: targetName,
		GhostID:    ghostID,
		AdminAddr:  adminAddr,
	}, nil
}

func (s *Service) stopManagedGhosts() {
	s.cluster.mu.Lock()
	nodes := make([]*managedGhost, 0, len(s.cluster.managed))
	for _, node := range s.cluster.managed {
		nodes = append(nodes, node)
	}
	s.cluster.managed = make(map[string]*managedGhost)
	s.cluster.mu.Unlock()

	for _, node := range nodes {
		logs.Infof("ghost.cluster child stopping target=%q addr=%q", node.name, node.cfg.AdminListenAddr)
		node.cancel()
		select {
		case <-node.done:
		case <-time.After(2 * time.Second):
		}
	}
}

func normalizeNodeSuffix(name string) string {
	raw := strings.ToLower(strings.TrimSpace(name))
	if raw == "" {
		return ""
	}
	var b strings.Builder
	for i := 0; i < len(raw); i++ {
		c := raw[i]
		isLower := c >= 'a' && c <= 'z'
		isDigit := c >= '0' && c <= '9'
		if isLower || isDigit {
			b.WriteByte(c)
			continue
		}
		if c == '.' || c == '-' || c == '_' || c == ' ' {
			b.WriteByte('.')
		}
	}
	out := strings.Trim(b.String(), ".")
	out = strings.ReplaceAll(out, "..", ".")
	return out
}
