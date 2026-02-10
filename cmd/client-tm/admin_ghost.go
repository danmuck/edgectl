package main

// admin_ghost.go defines the GhostAdmin interface and RemoteGhostAdmin TCP client implementation.

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/ghost"
	"github.com/danmuck/edgectl/internal/protocol/frame"
	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/seeds"
)

// GhostAdmin defines the client control boundary for one Ghost target.
type GhostAdmin interface {
	GhostID() string
	Address() string
	Status() (ghost.LifecycleStatus, error)
	ListSeeds() ([]seeds.SeedMetadata, error)
	Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error)
	ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error)
	RecentEvents(limit int) ([]ghost.EventEnv, error)
	Verification(limit int) ([]ghost.VerificationRecord, error)
	SpawnGhost(req ghost.SpawnGhostRequest) (ghost.SpawnGhostResult, error)
	Close() error
}

func NewRemoteGhostAdmin(addr string) *RemoteGhostAdmin {
	return &RemoteGhostAdmin{addr: strings.TrimSpace(addr)}
}

func (c *RemoteGhostAdmin) GhostID() string {
	status, err := c.Status()
	if err != nil {
		return ""
	}
	return status.GhostID
}

func (c *RemoteGhostAdmin) Address() string {
	return c.addr
}

func (c *RemoteGhostAdmin) Status() (ghost.LifecycleStatus, error) {
	var status ghost.LifecycleStatus
	if err := c.call(controlRequest{Action: "status"}, &status); err != nil {
		return ghost.LifecycleStatus{}, err
	}
	return status, nil
}

func (c *RemoteGhostAdmin) ListSeeds() ([]seeds.SeedMetadata, error) {
	var list []seeds.SeedMetadata
	if err := c.call(controlRequest{Action: "list_seeds"}, &list); err != nil {
		return nil, err
	}
	return list, nil
}

func (c *RemoteGhostAdmin) Execute(command GhostAdminCommand) (ghost.ExecutionState, ghost.EventEnv, error) {
	intentID := strings.TrimSpace(command.IntentID)
	if intentID == "" {
		intentID = fmt.Sprintf("intent.clienttm.%d", time.Now().UnixMilli())
	}
	commandID := fmt.Sprintf("cmd.clienttm.%d", time.Now().UnixNano())

	ghostID := "ghost.local"
	if status, err := c.Status(); err == nil {
		if id := strings.TrimSpace(status.GhostID); id != "" {
			ghostID = id
		}
	}

	commandFrame, err := session.EncodeCommandFrame(1, session.Command{
		CommandID:    commandID,
		IntentID:     intentID,
		GhostID:      ghostID,
		SeedSelector: strings.TrimSpace(command.SeedSelector),
		Operation:    strings.TrimSpace(command.Operation),
		Args:         cloneArgs(command.Args),
	})
	if err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}

	var out executeEnvelopeResponse
	if err := c.call(controlRequest{Action: "execute_envelope", CommandFrame: commandFrame}, &out); err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}
	fr, err := frame.ReadFrame(bytes.NewReader(out.EventFrame), frame.DefaultLimits())
	if err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}
	event, err := session.DecodeEventFrame(fr)
	if err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}

	execState, found, err := c.ExecutionByCommandID(strings.TrimSpace(event.CommandID))
	if err != nil {
		return ghost.ExecutionState{}, ghost.EventEnv{}, err
	}
	if !found {
		return ghost.ExecutionState{}, ghost.EventEnv{}, fmt.Errorf(
			"ghostctl: execution state not found for command_id=%q",
			strings.TrimSpace(event.CommandID),
		)
	}
	return execState, ghost.EventEnv{
		EventID:     event.EventID,
		CommandID:   event.CommandID,
		IntentID:    event.IntentID,
		GhostID:     event.GhostID,
		SeedID:      event.SeedID,
		Outcome:     event.Outcome,
		TimestampMS: event.TimestampMS,
	}, nil
}

func (c *RemoteGhostAdmin) ExecutionByCommandID(commandID string) (ghost.ExecutionState, bool, error) {
	var out executionLookupResponse
	req := controlRequest{
		Action:    "execution_by_command_id",
		CommandID: strings.TrimSpace(commandID),
	}
	if err := c.call(req, &out); err != nil {
		return ghost.ExecutionState{}, false, err
	}
	return out.Execution, out.Found, nil
}

func (c *RemoteGhostAdmin) RecentEvents(limit int) ([]ghost.EventEnv, error) {
	var out []ghost.EventEnv
	if err := c.call(controlRequest{Action: "recent_events", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (c *RemoteGhostAdmin) Verification(limit int) ([]ghost.VerificationRecord, error) {
	var out []ghost.VerificationRecord
	if err := c.call(controlRequest{Action: "verification", Limit: limit}, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// SpawnGhost asks a connected root Ghost to provision a child Ghost node.
func (c *RemoteGhostAdmin) SpawnGhost(req ghost.SpawnGhostRequest) (ghost.SpawnGhostResult, error) {
	var out ghost.SpawnGhostResult
	controlReq := controlRequest{
		Action: "spawn_ghost",
		Spawn:  req,
	}
	if err := c.call(controlReq, &out); err != nil {
		return ghost.SpawnGhostResult{}, err
	}
	return out, nil
}

// call sends one admin request to ghostctl and decodes the response payload.
func (c *RemoteGhostAdmin) call(req controlRequest, out any) error {
	if err := c.ensureConn(); err != nil {
		return err
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return err
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	payload = append(payload, '\n')
	if _, err := c.conn.Write(payload); err != nil {
		c.resetConn()
		return err
	}
	if err := c.conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
		return err
	}
	line, err := c.r.ReadBytes('\n')
	if err != nil {
		c.resetConn()
		return err
	}
	var resp controlResponse
	if err := json.Unmarshal(line, &resp); err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	if out == nil {
		return nil
	}
	if len(resp.Data) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Data, out)
}

func (c *RemoteGhostAdmin) ensureConn() error {
	if c.conn != nil {
		return nil
	}
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return err
	}
	c.conn = conn
	c.r = bufio.NewReader(conn)
	return nil
}

func (c *RemoteGhostAdmin) resetConn() {
	if c.conn != nil {
		_ = c.conn.Close()
	}
	c.conn = nil
	c.r = nil
}

// Close terminates the persistent admin connection for this target.
func (c *RemoteGhostAdmin) Close() error {
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	c.r = nil
	return err
}

func cloneArgs(in map[string]string) map[string]string {
	if len(in) == 0 {
		return map[string]string{}
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
