package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	seedpkg "github.com/danmuck/edgectl/internal/seed"
	"github.com/danmuck/edgectl/internal/services"
	logs "github.com/danmuck/smplog"
)

func TestGhostToSeedControlFlowLocalService(t *testing.T) {
	ghost := Appear("ghost-a", ":9000", nil)
	local := ghost.CreateLocalSeed("local", "/local/local")
	flow := &services.FlowService{}
	local.Registry.Register(flow)
	ghost.RegisterRoutesTMP()

	logGhostf("ghost/init: ghost=%s local-seed=%s base=/local/local", ghost.Name, local.ID)

	steps := []struct {
		action string
		want   string
	}{
		{action: "intent", want: "intent stored"},
		{action: "command", want: "command stored"},
		{action: "event", want: "event stored"},
	}

	for _, step := range steps {
		path := "/seeds/local/services/flow/actions/" + step.action
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rr := httptest.NewRecorder()
		ghost.HTTPRouter().ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("expected 200 for %s, got %d body=%s", step.action, rr.Code, rr.Body.String())
		}

		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode %s response: %v", step.action, err)
		}
		if body["status"] != "ok" {
			t.Fatalf("expected status ok for %s, got %#v", step.action, body["status"])
		}
		if body["output"] != step.want {
			t.Fatalf("expected output %q for %s, got %#v", step.want, step.action, body["output"])
		}

		logGhostf("ghost->seed rpc action=%s status=%v output=%v", step.action, body["status"], body["output"])
		logGhostSnapshot("after-"+step.action, flow.Snapshot())
	}
}

func TestGhostProxyToRemoteSeedRoute(t *testing.T) {
	remote := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"services":[{"name":"flow","actions":["intent","command","event"]}]}`))
	}))
	defer remote.Close()

	ghost := Appear("ghost-a", ":9000", nil)
	ghost.LoadSeeds([]seedpkg.Seed{{ID: "remote-1", Addr: remote.URL}})
	ghost.RegisterRoutesTMP()

	req := httptest.NewRequest(http.MethodGet, "/seeds/remote-1/services", nil)
	rr := httptest.NewRecorder()
	ghost.HTTPRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from proxy, got %d body=%s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	logGhostf("ghost->seed proxy route=/seeds/remote-1/services status=%d payload=%s", rr.Code, rr.Body.String())
	if _, ok := body["services"]; !ok {
		t.Fatalf("expected services key in proxied payload")
	}
}

func logGhostSnapshot(stage string, snap services.FlowSnapshot) {
	logGhostf("%s flow status next=%d intent=%d command=%d event=%d corr=%d",
		stage,
		snap.Status.NextMessageID,
		snap.Status.LastIntentID,
		snap.Status.LastCommandID,
		snap.Status.LastEventID,
		snap.Status.LastCorrelationID,
	)
	logGhostMessage(stage, "intent", snap.Intent)
	logGhostMessage(stage, "command", snap.Command)
	logGhostMessage(stage, "event", snap.Event)
}

func logGhostMessage(stage, label string, msg *services.MessageShape) {
	if msg == nil {
		logGhostf("%s %s-message=<nil>", stage, label)
		return
	}
	logGhostf("%s %s-message id=%d type=%d field-count=%d", stage, label, msg.MessageID, msg.MessageType, len(msg.Fields))
	for _, field := range msg.Fields {
		logGhostf("%s %s-field id=%d type=%d value=%s", stage, label, field.ID, field.Type, field.Value)
	}
}

func logGhostf(format string, v ...any) {
	logs.Logf(format, v...)
}
