package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"time"
)

const spawnGhostAction = "spawn_ghost"

type ghostControlRequest struct {
	Action string            `json:"action"`
	Spawn  SpawnGhostRequest `json:"spawn,omitempty"`
}

type ghostControlResponse struct {
	OK    bool            `json:"ok"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
}

// GhostAdminSpawner provisions local ghosts through an existing root Ghost admin endpoint.
type GhostAdminSpawner struct {
	adminAddr string
	timeout   time.Duration
}

// NewGhostAdminSpawner constructs a spawner bound to one root ghost admin address.
func NewGhostAdminSpawner(adminAddr string) *GhostAdminSpawner {
	return &GhostAdminSpawner{
		adminAddr: strings.TrimSpace(adminAddr),
		timeout:   5 * time.Second,
	}
}

// SpawnLocalGhost calls root ghost admin "spawn_ghost" for local provisioning.
func (s *GhostAdminSpawner) SpawnLocalGhost(ctx context.Context, req SpawnGhostRequest) (SpawnGhostResult, error) {
	addr := strings.TrimSpace(s.adminAddr)
	if addr == "" {
		return SpawnGhostResult{}, fmt.Errorf("mirage: ghost admin addr required")
	}
	dialer := net.Dialer{Timeout: s.timeout}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return SpawnGhostResult{}, err
	}
	defer conn.Close()

	line, err := json.Marshal(ghostControlRequest{
		Action: spawnGhostAction,
		Spawn:  req,
	})
	if err != nil {
		return SpawnGhostResult{}, err
	}
	line = append(line, '\n')
	_ = conn.SetWriteDeadline(time.Now().Add(s.timeout))
	if _, err := conn.Write(line); err != nil {
		return SpawnGhostResult{}, err
	}

	_ = conn.SetReadDeadline(time.Now().Add(s.timeout))
	respLine, err := bufio.NewReader(conn).ReadBytes('\n')
	if err != nil {
		return SpawnGhostResult{}, err
	}
	var resp ghostControlResponse
	if err := json.Unmarshal(respLine, &resp); err != nil {
		return SpawnGhostResult{}, err
	}
	if !resp.OK {
		return SpawnGhostResult{}, fmt.Errorf("mirage: spawn_ghost failed: %s", strings.TrimSpace(resp.Error))
	}
	var out SpawnGhostResult
	if len(resp.Data) > 0 {
		if err := json.Unmarshal(resp.Data, &out); err != nil {
			return SpawnGhostResult{}, err
		}
	}
	return out, nil
}
