package mirage

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestGhostAdminSpawnerSpawnLocalGhost(t *testing.T) {
	testlog.Start(t)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	done := make(chan struct{})
	data := mustJSON(t, SpawnGhostResult{
		TargetName: "ghost.local.edge.1",
		GhostID:    "ghost.local.edge.1",
		AdminAddr:  "127.0.0.1:7119",
	})
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
		if req.Action != spawnGhostAction {
			return
		}
		resp := ghostControlResponse{
			OK:   true,
			Data: data,
		}
		payload, _ := json.Marshal(resp)
		payload = append(payload, '\n')
		_, _ = conn.Write(payload)
	}()

	spawner := NewGhostAdminSpawner(ln.Addr().String())
	out, err := spawner.SpawnLocalGhost(context.Background(), SpawnGhostRequest{
		TargetName: "edge-1",
		AdminAddr:  "127.0.0.1:7119",
	})
	if err != nil {
		t.Fatalf("spawn local ghost: %v", err)
	}
	if out.GhostID != "ghost.local.edge.1" {
		t.Fatalf("unexpected output: %+v", out)
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
