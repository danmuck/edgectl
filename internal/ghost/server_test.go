package ghost

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

type stubService struct {
	name    string
	actions map[string]seeds.Action
}

func (s stubService) Name() string                     { return s.name }
func (s stubService) Status() (any, error)             { return "ok", nil }
func (s stubService) Actions() map[string]seeds.Action { return s.actions }

func TestExecuteActionOutputsAndErrors(t *testing.T) {
	s := Attach("ghost-a", nil, "", nil)
	s.Registry.Register(stubService{
		name: "svc",
		actions: map[string]seeds.Action{
			"ok":  func() (string, error) { return "done", nil },
			"err": func() (string, error) { return "", errors.New("boom") },
		},
	})

	out, err := s.ExecuteAction("svc", "ok")
	if err != nil || out != "done" {
		t.Fatalf("expected successful action output, out=%q err=%v", out, err)
	}
	logs.Logf("ghost/execute: seed=svc action=ok output=%q", out)

	if _, err := s.ExecuteAction("svc", "missing"); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("expected ErrActionNotFound, got %v", err)
	}
	logs.Logf("ghost/execute: missing action rejected")

	if _, err := s.ExecuteAction("missing", "ok"); !errors.Is(err, ErrSeedNotFound) {
		t.Fatalf("expected ErrSeedNotFound, got %v", err)
	}
	logs.Logf("ghost/execute: missing seed rejected")

	if _, err := s.ExecuteAction("svc", "err"); err == nil {
		t.Fatalf("expected seed action error")
	}
	logs.Logf("ghost/execute: action error path surfaced")
}

func TestRegisterRoutesActionIncludesOutput(t *testing.T) {
	s := Appear("ghost-a", ":9001", nil)
	s.Registry.Register(stubService{
		name: "svc",
		actions: map[string]seeds.Action{
			"ok": func() (string, error) { return "ran", nil },
		},
	})
	s.RegisterRoutes()

	req := httptest.NewRequest(http.MethodPost, "/seeds/svc/actions/ok", nil)
	rr := httptest.NewRecorder()
	s.HTTPRouter().ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if body["status"] != "ok" || body["output"] != "ran" {
		t.Fatalf("unexpected response body: %#v", body)
	}
	logs.Logf("ghost/http: POST /seeds/svc/actions/ok status=%d output=%v", rr.Code, body["output"])
}
