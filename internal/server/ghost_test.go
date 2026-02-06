package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/protocol"
	seedpkg "github.com/danmuck/edgectl/internal/seed"
	"github.com/danmuck/edgectl/internal/services"
	logs "github.com/danmuck/smplog"
)

func TestGhostToSeedControlFlowLocalService(t *testing.T) {
	ghost := Appear("ghost_srv", ":9000", nil)
	local := ghost.CreateLocalSeed("seed_srv", "/local/seed_srv")
	flow := &services.FlowService{}
	local.Registry.Register(flow)
	ghost.RegisterRoutesTMP()

	logShape("ghost_contract", map[string]any{
		"topology": map[string]any{
			"ghost_servers": 1,
			"seed_servers":  1,
			"flow_path":     "ghost -> seed.flow_service",
		},
		"protocol": map[string]any{
			"magic_label": "EDGE",
			"magic_value": fmt.Sprintf("0x%08X", protocol.Magic),
			"version":     protocol.Version,
		},
		"ghost_struct": map[string]any{
			"name": ghost.Name,
			"local": map[string]any{
				local.ID: local.ID,
			},
		},
		"seed_struct": map[string]any{
			"id":   local.ID,
			"addr": local.Addr,
		},
	})

	steps := []struct {
		action string
		want   string
	}{
		{action: "intent", want: "intent stored"},
		{action: "command", want: "command stored"},
		{action: "event", want: "event stored"},
	}

	for _, step := range steps {
		path := "/seeds/seed_srv/services/flow/actions/" + step.action
		logShape("ghost_request", map[string]any{
			"method": "POST",
			"path":   path,
			"from":   "ghost",
			"to":     "seed(seed_srv).flow",
		})

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

		logShape("ghost_response", map[string]any{
			"status_code": rr.Code,
			"output":      body["output"],
			"status":      body["status"],
		})
		logShape("ghost_flow_snapshot", snapshotShape(flow.Snapshot()))
	}
}

func TestGhostProxyToRemoteSeedRoute(t *testing.T) {
	remote := newTestServerOrSkip(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/services" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"services":[{"name":"flow","actions":["intent","command","event"]}]}`))
	}))
	defer remote.Close()

	ghost := Appear("ghost_srv", ":9000", nil)
	ghost.LoadSeeds([]seedpkg.Seed{{ID: "seed_remote", Addr: remote.URL}})
	ghost.RegisterRoutesTMP()

	logShape("ghost_proxy_request", map[string]any{
		"topology": "ghost_servers=1 seed_servers=1 (proxy to remote seed)",
		"method":   "GET",
		"path":     "/seeds/seed_remote/services",
		"from":     "ghost",
		"to":       "seed(seed_remote)",
	})

	req := httptest.NewRequest(http.MethodGet, "/seeds/seed_remote/services", nil)
	rr := httptest.NewRecorder()
	ghost.HTTPRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 from proxy, got %d body=%s", rr.Code, rr.Body.String())
	}

	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode proxy response: %v", err)
	}
	if _, ok := body["services"]; !ok {
		t.Fatalf("expected services key in proxied payload")
	}

	logShape("ghost_proxy_response", map[string]any{
		"status_code": rr.Code,
		"services":    servicesListShape(body["services"]),
	})
}

func newTestServerOrSkip(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	var server *httptest.Server
	func() {
		defer func() {
			if r := recover(); r != nil {
				server = nil
			}
		}()
		server = httptest.NewServer(handler)
	}()
	if server == nil {
		t.Skip("skipping proxy-listener test in restricted environment")
	}
	return server
}

func snapshotShape(snap services.FlowSnapshot) map[string]any {
	return map[string]any{
		"status": map[string]any{
			"next_message_id":     snap.Status.NextMessageID,
			"last_intent_id":      snap.Status.LastIntentID,
			"last_command_id":     snap.Status.LastCommandID,
			"last_event_id":       snap.Status.LastEventID,
			"last_correlation_id": snap.Status.LastCorrelationID,
		},
		"intent":  messageShapeMap(snap.Intent),
		"command": messageShapeMap(snap.Command),
		"event":   messageShapeMap(snap.Event),
	}
}

func messageShapeMap(msg *services.MessageShape) map[string]any {
	if msg == nil {
		return map[string]any{"value": "nil"}
	}
	fields := map[string]any{}
	for i, f := range msg.Fields {
		fields[fmt.Sprintf("field_%d", i+1)] = map[string]any{
			"id":            f.ID,
			"field_label":   flowFieldLabel(f.ID),
			"represents":    flowFieldMeaning(f.ID),
			"kind":          int(f.Type),
			"kind_label":    fieldTypeLabel(f.Type),
			"decoded_value": f.Value,
		}
	}
	return map[string]any{
		"message_id":         msg.MessageID,
		"message_type":       int(msg.MessageType),
		"message_type_label": messageTypeLabel(msg.MessageType),
		"fields":             fields,
	}
}

func messageTypeLabel(mt protocol.MessageType) string {
	switch mt {
	case protocol.MessageIntent:
		return "intent"
	case protocol.MessageCommand:
		return "command"
	case protocol.MessageEvent:
		return "event"
	default:
		return "unknown"
	}
}

func fieldTypeLabel(ft protocol.FieldType) string {
	switch ft {
	case protocol.FieldUint8:
		return "uint8"
	case protocol.FieldUint16:
		return "uint16"
	case protocol.FieldUint32:
		return "uint32"
	case protocol.FieldUint64:
		return "uint64"
	case protocol.FieldBool:
		return "bool"
	case protocol.FieldString:
		return "string"
	case protocol.FieldBytes:
		return "bytes"
	default:
		return "unknown"
	}
}

func flowFieldLabel(id uint16) string {
	switch id {
	case 1:
		return "intent_name"
	case 2:
		return "intent_target"
	case 10:
		return "command_name"
	case 11:
		return "command_target"
	case 12:
		return "correlation_id"
	case 20:
		return "event_name"
	case 21:
		return "event_status"
	default:
		return "unknown_field"
	}
}

func flowFieldMeaning(id uint16) string {
	switch id {
	case 1:
		return "requested high-level operation"
	case 2:
		return "target node or domain for intent"
	case 10:
		return "executable command derived from intent"
	case 11:
		return "execution target for command"
	case 12:
		return "links command/event back to originating intent"
	case 20:
		return "event emitted after command execution"
	case 21:
		return "event outcome status"
	default:
		return "unmapped semantic meaning"
	}
}

func servicesListShape(raw any) map[string]any {
	shape := map[string]any{}
	list, ok := raw.([]any)
	if !ok {
		shape["value"] = fmt.Sprintf("%v", raw)
		return shape
	}
	for i, item := range list {
		entry := map[string]any{}
		itemMap, ok := item.(map[string]any)
		if !ok {
			entry["value"] = fmt.Sprintf("%v", item)
		} else {
			for k, v := range itemMap {
				switch vv := v.(type) {
				case []any:
					actionShape := map[string]any{}
					for j, av := range vv {
						actionShape[fmt.Sprintf("item_%d", j+1)] = fmt.Sprintf("%v", av)
					}
					entry[k] = actionShape
				default:
					entry[k] = vv
				}
			}
		}
		shape[fmt.Sprintf("item_%d", i+1)] = entry
	}
	return shape
}

func logShape(label string, payload map[string]any) {
	lines := []string{label + ":"}
	lines = append(lines, formatKV(payload, 1)...)
	logs.Log(strings.Join(lines, "\n"))
}

func formatKV(m map[string]any, depth int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	pad := strings.Repeat("    ", depth)
	out := make([]string, 0)
	for _, k := range keys {
		v := m[k]
		switch x := v.(type) {
		case map[string]any:
			out = append(out, fmt.Sprintf("%s%s:", pad, k))
			out = append(out, formatKV(x, depth+1)...)
		case string:
			out = append(out, fmt.Sprintf("%s%s: %q", pad, k, x))
		default:
			out = append(out, fmt.Sprintf("%s%s: %v", pad, k, x))
		}
	}
	return out
}
