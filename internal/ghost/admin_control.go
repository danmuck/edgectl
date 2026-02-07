package ghost

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/schema"
	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

// AdminCommand is the external admin execution request payload.
type AdminCommand struct {
	IntentID     string            `json:"intent_id"`
	SeedSelector string            `json:"seed_selector"`
	Operation    string            `json:"operation"`
	Args         map[string]string `json:"args"`
}

// VerificationRecord captures command->event custody fields for client verification views.
type VerificationRecord struct {
	RequestID          string `json:"request_id"`
	TraceID            string `json:"trace_id"`
	CommandMessageID   uint64 `json:"command_message_id"`
	CommandMessageType uint32 `json:"command_message_type"`
	EventMessageType   uint32 `json:"event_message_type"`
	CommandID          string `json:"command_id"`
	ExecutionID        string `json:"execution_id"`
	EventID            string `json:"event_id"`
	GhostID            string `json:"ghost_id"`
	SeedID             string `json:"seed_id"`
	Operation          string `json:"operation"`
	Outcome            string `json:"outcome"`
	SeedStatus         string `json:"seed_status"`
	ExitCode           int32  `json:"exit_code"`
	TimestampMS        uint64 `json:"timestamp_ms"`
	Status             string `json:"status"`
}

// controlRequest is one admin action envelope consumed by ghostctl.
type controlRequest struct {
	Action    string            `json:"action"`
	Limit     int               `json:"limit,omitempty"`
	CommandID string            `json:"command_id,omitempty"`
	Command   AdminCommand      `json:"command,omitempty"`
	Spawn     SpawnGhostRequest `json:"spawn,omitempty"`
}

// controlResponse is one admin action result envelope emitted by ghostctl.
type controlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
	Data  any    `json:"data,omitempty"`
}

// ExecuteAdminCommand maps one external admin request into Ghost command execution.
func (s *Service) ExecuteAdminCommand(cmd AdminCommand) (ExecutionState, EventEnv, error) {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()

	status := s.server.Status()
	messageID := s.adminSeq.Add(1)
	commandID := fmt.Sprintf("cmd.%s.%d", status.GhostID, messageID)
	intentID := strings.TrimSpace(cmd.IntentID)
	if intentID == "" {
		intentID = fmt.Sprintf("intent.%s.%d", status.GhostID, messageID)
	}

	env := CommandEnv{
		MessageID:    messageID,
		CommandID:    commandID,
		IntentID:     intentID,
		GhostID:      status.GhostID,
		SeedSelector: strings.TrimSpace(cmd.SeedSelector),
		Operation:    strings.TrimSpace(cmd.Operation),
		Args:         cloneArgs(cmd.Args),
	}

	event, err := s.server.HandleCommandAndExecute(env)
	if err != nil {
		return ExecutionState{}, EventEnv{}, err
	}
	state, ok := s.server.ExecutionByCommandID(commandID)
	if !ok {
		return ExecutionState{}, EventEnv{}, fmt.Errorf("ghost: missing execution state for command_id=%q", commandID)
	}

	s.adminEvents = append(s.adminEvents, event)
	rec := VerificationRecord{
		RequestID:          fmt.Sprintf("req.%s.%d", status.GhostID, messageID),
		TraceID:            fmt.Sprintf("trace.%s.%d", status.GhostID, messageID),
		CommandMessageID:   messageID,
		CommandMessageType: schema.MsgCommand,
		EventMessageType:   schema.MsgEvent,
		CommandID:          state.CommandID,
		ExecutionID:        state.ExecutionID,
		EventID:            event.EventID,
		GhostID:            status.GhostID,
		SeedID:             event.SeedID,
		Operation:          state.Operation,
		Outcome:            event.Outcome,
		SeedStatus:         state.SeedResult.Status,
		ExitCode:           state.SeedResult.ExitCode,
		TimestampMS:        event.TimestampMS,
		Status:             event.Outcome,
	}
	s.verificationEvents = append(s.verificationEvents, rec)
	return state, event, nil
}

func (s *Service) ListSeeds() []seeds.SeedMetadata {
	return s.server.SeedMetadata()
}

// ExecutionByCommandID proxies execution lookup by command id.
func (s *Service) ExecutionByCommandID(commandID string) (ExecutionState, bool) {
	return s.server.ExecutionByCommandID(commandID)
}

func (s *Service) RecentAdminEvents(limit int) []EventEnv {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	if len(s.adminEvents) <= limit {
		out := make([]EventEnv, len(s.adminEvents))
		copy(out, s.adminEvents)
		return out
	}
	out := make([]EventEnv, limit)
	copy(out, s.adminEvents[len(s.adminEvents)-limit:])
	return out
}

// VerificationView returns a bounded list of correlation records for protocol inspection.
func (s *Service) VerificationView(limit int) []VerificationRecord {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	if limit <= 0 {
		limit = 20
	}
	if len(s.verificationEvents) <= limit {
		out := make([]VerificationRecord, len(s.verificationEvents))
		copy(out, s.verificationEvents)
		return out
	}
	out := make([]VerificationRecord, limit)
	copy(out, s.verificationEvents[len(s.verificationEvents)-limit:])
	return out
}

// serveAdminControl exposes a TCP JSON request/response endpoint for client-tm.
func (s *Service) serveAdminControl(ctx context.Context, addr string) error {
	ln, err := net.Listen("tcp", strings.TrimSpace(addr))
	if err != nil {
		return err
	}
	defer ln.Close()
	logs.Infof("ghost.admin listening addr=%q", ln.Addr().String())

	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.handleAdminConn(conn)
	}
}

// handleAdminConn decodes one request per line and writes one response per line.
func (s *Service) handleAdminConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr().String()
	active := s.adminClientCount.Add(1)
	logs.Infof("ghost.admin client connected remote=%q active_clients=%d", remote, active)
	defer func() {
		remaining := s.adminClientCount.Add(-1)
		logs.Infof("ghost.admin client disconnected remote=%q active_clients=%d", remote, remaining)
	}()

	reader := bufio.NewReader(conn)
	for {
		_ = conn.SetReadDeadline(time.Now().Add(30 * time.Second))
		line, err := reader.ReadBytes('\n')
		if err != nil {
			if err != io.EOF {
				logs.Warnf("ghost.admin read err=%v", err)
			}
			return
		}
		var req controlRequest
		if err := json.Unmarshal(line, &req); err != nil {
			_ = writeControlResponse(conn, controlResponse{OK: false, Error: err.Error()})
			continue
		}
		resp := s.handleControlRequest(req)
		if err := writeControlResponse(conn, resp); err != nil {
			logs.Warnf("ghost.admin write err=%v", err)
			return
		}
	}
}

// handleControlRequest dispatches RPC-like admin actions to service methods.
func (s *Service) handleControlRequest(req controlRequest) controlResponse {
	switch req.Action {
	case "status":
		return controlResponse{OK: true, Data: s.server.Status()}
	case "list_seeds":
		return controlResponse{OK: true, Data: s.ListSeeds()}
	case "execute":
		state, event, err := s.ExecuteAdminCommand(req.Command)
		if err != nil {
			return controlResponse{OK: false, Error: err.Error()}
		}
		return controlResponse{
			OK: true,
			Data: map[string]any{
				"execution": state,
				"event":     event,
			},
		}
	case "execution_by_command_id":
		state, ok := s.ExecutionByCommandID(req.CommandID)
		return controlResponse{
			OK: true,
			Data: map[string]any{
				"found":     ok,
				"execution": state,
			},
		}
	case "recent_events":
		return controlResponse{OK: true, Data: s.RecentAdminEvents(req.Limit)}
	case "verification":
		return controlResponse{OK: true, Data: s.VerificationView(req.Limit)}
	case "spawn_ghost":
		out, err := s.SpawnManagedGhost(req.Spawn)
		if err != nil {
			return controlResponse{OK: false, Error: err.Error()}
		}
		return controlResponse{OK: true, Data: out}
	default:
		return controlResponse{OK: false, Error: fmt.Sprintf("unknown action: %s", req.Action)}
	}
}

func writeControlResponse(w io.Writer, resp controlResponse) error {
	payload, err := json.Marshal(resp)
	if err != nil {
		return err
	}
	payload = append(payload, '\n')
	_, err = w.Write(payload)
	return err
}
