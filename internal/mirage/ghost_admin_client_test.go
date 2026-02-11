package mirage

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/danmuck/edgectl/internal/protocol/frame"
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
			case executeEnvelopeAction:
				fr, err := frame.ReadFrame(bytes.NewReader(req.CommandFrame), frame.DefaultLimits())
				if err != nil {
					_ = conn.Close()
					return
				}
				cmd, err := session.DecodeCommandFrame(fr)
				if err != nil {
					_ = conn.Close()
					return
				}
				eventFrame, err := session.EncodeEventFrame(fr.Header.MessageID, session.Event{
					EventID:     "evt.cmd.intent.1.1",
					CommandID:   cmd.CommandID,
					IntentID:    cmd.IntentID,
					GhostID:     cmd.GhostID,
					SeedID:      cmd.SeedSelector,
					Outcome:     "success",
					TimestampMS: uint64(time.Now().UnixMilli()),
				})
				if err != nil {
					_ = conn.Close()
					return
				}
				resp = ghostControlResponse{
					OK: true,
					Data: mustJSON(t, ghostExecuteEnvelopeResponse{
						EventFrame: eventFrame,
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
			OK:   true,
			Data: mustJSON(t, ghostExecuteEnvelopeResponse{}),
		}
		if req.Action != executeEnvelopeAction {
			return
		}
		fr, err := frame.ReadFrame(bytes.NewReader(req.CommandFrame), frame.DefaultLimits())
		if err != nil {
			return
		}
		cmd, err := session.DecodeCommandFrame(fr)
		if err != nil {
			return
		}
		eventFrame, err := session.EncodeEventFrame(fr.Header.MessageID, session.Event{
			EventID:     "evt." + cmd.CommandID,
			CommandID:   cmd.CommandID,
			IntentID:    cmd.IntentID,
			GhostID:     cmd.GhostID,
			SeedID:      cmd.SeedSelector,
			Outcome:     "success",
			TimestampMS: uint64(time.Now().UnixMilli()),
		})
		if err != nil {
			return
		}
		resp.Data = mustJSON(t, ghostExecuteEnvelopeResponse{EventFrame: eventFrame})
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

func TestGhostControlClientExecuteAdminCommandWithAuthToken(t *testing.T) {
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
		if req.Action != executeEnvelopeAction {
			return
		}

		fr, err := frame.ReadFrame(bytes.NewReader(req.CommandFrame), frame.DefaultLimits())
		if err != nil {
			return
		}
		if string(fr.Auth) != "phase6-token" {
			return
		}
		cmd, err := session.DecodeCommandFrame(fr)
		if err != nil {
			return
		}
		eventFrame, err := session.EncodeEventFrame(fr.Header.MessageID, session.Event{
			EventID:     "evt." + cmd.CommandID,
			CommandID:   cmd.CommandID,
			IntentID:    cmd.IntentID,
			GhostID:     cmd.GhostID,
			SeedID:      cmd.SeedSelector,
			Outcome:     "success",
			TimestampMS: uint64(time.Now().UnixMilli()),
		})
		if err != nil {
			return
		}
		resp := ghostControlResponse{
			OK:   true,
			Data: mustJSON(t, ghostExecuteEnvelopeResponse{EventFrame: eventFrame}),
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	client := NewGhostControlClient(ln.Addr().String()).WithCommandFrameAuthToken("phase6-token")
	event, err := client.ExecuteAdminCommand(context.Background(), ghostAdminCommand{
		CommandID:    "cmd.intent.auth.1",
		IntentID:     "intent.auth.1",
		SeedSelector: "seed.flow",
		Operation:    "status",
	})
	if err != nil {
		t.Fatalf("execute command: %v", err)
	}
	if event.CommandID != "cmd.intent.auth.1" || event.Outcome != "success" {
		t.Fatalf("unexpected event: %+v", event)
	}
	<-done
}

func TestGhostControlClientListSeedCatalog(t *testing.T) {
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
		if req.Action != listSeedCatalogAction {
			return
		}
		resp := ghostControlResponse{
			OK: true,
			Data: mustJSON(t, []map[string]any{
				{
					"metadata": map[string]any{
						"id":          "seed.flow",
						"name":        "Flow",
						"description": "Deterministic control-flow seed",
					},
					"operations": []map[string]any{
						{"name": "status", "description": "status", "idempotent": true},
					},
					"command_catalog": []map[string]any{
						{
							"id":            "seed.flow.status",
							"label":         "Flow Status",
							"description":   "Read deterministic flow status.",
							"seed_selector": "seed.flow",
							"operation":     "status",
							"args": []map[string]any{
								{"key": "name", "prompt": "name", "required": false},
							},
							"default_blocking": false,
						},
					},
				},
			}),
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	client := NewGhostControlClient(ln.Addr().String())
	catalog, err := client.ListSeedCatalog(context.Background())
	if err != nil {
		t.Fatalf("list seed catalog: %v", err)
	}
	if len(catalog) != 1 {
		t.Fatalf("unexpected catalog size: %d", len(catalog))
	}
	if catalog[0].Metadata.ID != "seed.flow" {
		t.Fatalf("unexpected seed id: %+v", catalog[0].Metadata)
	}
	if len(catalog[0].Operations) != 1 || catalog[0].Operations[0].Name != "status" {
		t.Fatalf("unexpected operations: %+v", catalog[0].Operations)
	}
	if len(catalog[0].CommandCatalog) != 1 || catalog[0].CommandCatalog[0].SeedSelector != "seed.flow" {
		t.Fatalf("unexpected command catalog: %+v", catalog[0].CommandCatalog)
	}
	<-done
}

// TestGhostSeedBuildlogStorePersistSeedFSIncludesCommandID verifies seed.fs buildlog writes
// emit protocol-valid command envelopes with non-empty command_id.
func TestGhostSeedBuildlogStorePersistSeedFSIncludesCommandID(t *testing.T) {
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
		if req.Action != executeEnvelopeAction {
			return
		}

		fr, err := frame.ReadFrame(bytes.NewReader(req.CommandFrame), frame.DefaultLimits())
		if err != nil {
			return
		}
		cmd, err := session.DecodeCommandFrame(fr)
		if err != nil {
			return
		}
		if strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		if !strings.HasPrefix(cmd.CommandID, "cmd.intent.mirage.buildlog.") {
			return
		}
		if cmd.IntentID != "intent.mirage.buildlog" {
			return
		}
		if cmd.SeedSelector != "seed.fs" || cmd.Operation != "write" {
			return
		}
		if cmd.Args["path"] != "local/buildlogs/test.log" {
			return
		}
		if cmd.Args["content"] != "{\"ok\":true}" {
			return
		}

		eventFrame, err := session.EncodeEventFrame(fr.Header.MessageID, session.Event{
			EventID:     "evt." + cmd.CommandID,
			CommandID:   cmd.CommandID,
			IntentID:    cmd.IntentID,
			GhostID:     cmd.GhostID,
			SeedID:      cmd.SeedSelector,
			Outcome:     "success",
			TimestampMS: uint64(time.Now().UnixMilli()),
		})
		if err != nil {
			return
		}
		resp := ghostControlResponse{
			OK:   true,
			Data: mustJSON(t, ghostExecuteEnvelopeResponse{EventFrame: eventFrame}),
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	store := NewGhostSeedBuildlogStore(NewGhostControlClient(ln.Addr().String()), "seed.fs")
	if err := store.Persist(context.Background(), "local/buildlogs/test.log", "{\"ok\":true}"); err != nil {
		t.Fatalf("persist buildlog seed.fs: %v", err)
	}
	<-done
}

// TestGhostSeedBuildlogStorePersistSeedKVIncludesCommandID verifies seed.kv buildlog writes
// emit protocol-valid command envelopes with non-empty command_id.
func TestGhostSeedBuildlogStorePersistSeedKVIncludesCommandID(t *testing.T) {
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
		if req.Action != executeEnvelopeAction {
			return
		}

		fr, err := frame.ReadFrame(bytes.NewReader(req.CommandFrame), frame.DefaultLimits())
		if err != nil {
			return
		}
		cmd, err := session.DecodeCommandFrame(fr)
		if err != nil {
			return
		}
		if strings.TrimSpace(cmd.CommandID) == "" {
			return
		}
		if !strings.HasPrefix(cmd.CommandID, "cmd.intent.mirage.buildlog.") {
			return
		}
		if cmd.IntentID != "intent.mirage.buildlog" {
			return
		}
		if cmd.SeedSelector != "seed.kv" || cmd.Operation != "put" {
			return
		}
		if cmd.Args["key"] != "local/buildlogs/test.log" {
			return
		}
		if cmd.Args["value"] != "{\"ok\":true}" {
			return
		}

		eventFrame, err := session.EncodeEventFrame(fr.Header.MessageID, session.Event{
			EventID:     "evt." + cmd.CommandID,
			CommandID:   cmd.CommandID,
			IntentID:    cmd.IntentID,
			GhostID:     cmd.GhostID,
			SeedID:      cmd.SeedSelector,
			Outcome:     "success",
			TimestampMS: uint64(time.Now().UnixMilli()),
		})
		if err != nil {
			return
		}
		resp := ghostControlResponse{
			OK:   true,
			Data: mustJSON(t, ghostExecuteEnvelopeResponse{EventFrame: eventFrame}),
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	store := NewGhostSeedBuildlogStore(NewGhostControlClient(ln.Addr().String()), "seed.kv")
	if err := store.Persist(context.Background(), "local/buildlogs/test.log", "{\"ok\":true}"); err != nil {
		t.Fatalf("persist buildlog seed.kv: %v", err)
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
