package seed

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/danmuck/edgectl/internal/services"
	logs "github.com/danmuck/smplog"
)

type stubService struct {
	name    string
	actions map[string]services.Action
}

func (s stubService) Name() string                        { return s.name }
func (s stubService) Status() (any, error)                { return "ok", nil }
func (s stubService) Actions() map[string]services.Action { return s.actions }

func TestExecuteActionOutputsAndErrors(t *testing.T) {
	s := Attach("seed-a", nil, "", nil)
	s.Registry.Register(stubService{
		name: "svc",
		actions: map[string]services.Action{
			"ok":  func() (string, error) { return "done", nil },
			"err": func() (string, error) { return "", errors.New("boom") },
		},
	})

	out, err := s.ExecuteAction("svc", "ok")
	if err != nil || out != "done" {
		t.Fatalf("expected successful action output, out=%q err=%v", out, err)
	}
	logs.Logf("seed/execute: service=svc action=ok output=%q", out)

	if _, err := s.ExecuteAction("svc", "missing"); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("expected ErrActionNotFound, got %v", err)
	}
	logs.Logf("seed/execute: missing action rejected")

	if _, err := s.ExecuteAction("missing", "ok"); !errors.Is(err, ErrServiceNotFound) {
		t.Fatalf("expected ErrServiceNotFound, got %v", err)
	}
	logs.Logf("seed/execute: missing service rejected")

	if _, err := s.ExecuteAction("svc", "err"); err == nil {
		t.Fatalf("expected service action error")
	}
	logs.Logf("seed/execute: action error path surfaced")
}

func TestRegisterRoutesActionIncludesOutput(t *testing.T) {
	s := Appear("seed-a", ":9001", nil)
	s.Registry.Register(stubService{
		name: "svc",
		actions: map[string]services.Action{
			"ok": func() (string, error) { return "ran", nil },
		},
	})
	s.RegisterRoutes()

	req := httptest.NewRequest(http.MethodPost, "/services/svc/actions/ok", nil)
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
	logs.Logf("seed/http: POST /services/svc/actions/ok status=%d output=%v", rr.Code, body["output"])
}
