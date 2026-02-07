package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/session"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestGhostControlClientSpawnAndExecute(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		for i := 0; i < 2; i++ {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			reader := bufio.NewReader(conn)
			line, err := reader.ReadBytes('\n')
			if err != nil {
				_ = conn.Close()
				return
			}
			var req ghostControlRequest
			if err := json.Unmarshal(line, &req); err != nil {
				_ = conn.Close()
				return
			}
			var resp ghostControlResponse
			switch req.Action {
			case spawnGhostAction:
				resp = ghostControlResponse{
					OK: true,
					Data: mustJSON(t, SpawnGhostResult{
						TargetName: "ghost.local.edge.1",
						GhostID:    "ghost.local.edge.1",
						AdminAddr:  "127.0.0.1:7119",
					}),
				}
			case executeAction:
				resp = ghostControlResponse{
					OK: true,
					Data: mustJSON(t, ghostExecuteResponse{
						Event: ghostEvent{
							EventID:     "evt.cmd.intent.1.1",
							CommandID:   req.Command.CommandID,
							IntentID:    req.Command.IntentID,
							GhostID:     "ghost.local",
							SeedID:      req.Command.SeedSelector,
							Outcome:     "success",
							TimestampMS: uint64(time.Now().UnixMilli()),
						},
					}),
				}
			}
			payload, _ := json.Marshal(resp)
			payload = append(payload, '\n')
			_, _ = conn.Write(payload)
			_ = conn.Close()
		}
	}()

	client := NewGhostControlClient(ln.Addr().String())
	spawnOut, err := client.SpawnLocalGhost(context.Background(), SpawnGhostRequest{
		TargetName: "edge-1",
		AdminAddr:  "127.0.0.1:7119",
	})
	if err != nil {
		t.Fatalf("spawn local ghost: %v", err)
	}
	if spawnOut.GhostID != "ghost.local.edge.1" {
		t.Fatalf("unexpected spawn output: %+v", spawnOut)
	}

	event, err := client.ExecuteAdminCommand(context.Background(), ghostAdminCommand{
		CommandID:    "cmd.intent.1.1",
		IntentID:     "intent.1",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if event.CommandID != "cmd.intent.1.1" || event.Outcome != "success" {
		t.Fatalf("unexpected event: %+v", event)
	}
	<-done
}

func TestGhostAdminCommandExecutor(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	go func() {
		defer close(done)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		reader := bufio.NewReader(conn)
		line, err := reader.ReadBytes('\n')
		if err != nil {
			return
		}
		var req ghostControlRequest
		if err := json.Unmarshal(line, &req); err != nil {
			return
		}
		resp := ghostControlResponse{
			OK: true,
			Data: mustJSON(t, ghostExecuteResponse{
				Event: ghostEvent{
					EventID:     "evt." + req.Command.CommandID,
					CommandID:   req.Command.CommandID,
					IntentID:    req.Command.IntentID,
					GhostID:     "ghost.local",
					SeedID:      req.Command.SeedSelector,
					Outcome:     "success",
					TimestampMS: uint64(time.Now().UnixMilli()),
				},
			}),
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	exec := NewGhostAdminCommandExecutor(NewGhostControlClient(ln.Addr().String()))
	event, err := exec.ExecuteCommand(context.Background(), session.Command{
		CommandID:    "cmd.intent.2.1",
		IntentID:     "intent.2",
		GhostID:      "ghost.local",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if event.CommandID != "cmd.intent.2.1" || event.Outcome != "success" {
		t.Fatalf("unexpected event: %+v", event)
	}
	<-done
}

func mustJSON(t *testing.T, v any) json.RawMessage {
	t.Helper()
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return out
}
