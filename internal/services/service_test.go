package services

import (
	"fmt"
	"io"
	"strings"
	"testing"

	logs "github.com/danmuck/smplog"
)

type fakeService struct {
	name    string
	actions map[string]Action
}

func (f fakeService) Name() string               { return f.name }
func (f fakeService) Status() (any, error)       { return "ok", nil }
func (f fakeService) Actions() map[string]Action { return f.actions }

type fakeRunner struct {
	calls []string
}

func (r *fakeRunner) Run(cmd string, args ...string) (string, error) {
	entry := strings.TrimSpace(fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")))
	r.calls = append(r.calls, entry)
	return "ran:" + entry, nil
}

func (r *fakeRunner) RunStreaming(cmd string, args []string, stdout, stderr io.Writer) error {
	entry := strings.TrimSpace(fmt.Sprintf("%s %s", cmd, strings.Join(args, " ")))
	r.calls = append(r.calls, "stream:"+entry)
	return nil
}

func TestServiceRegistrySnapshotSemantics(t *testing.T) {
	registry := NewServiceRegistry()
	service := fakeService{
		name: "svc-a",
		actions: map[string]Action{
			"ping": func() (string, error) { return "pong", nil },
		},
	}
	registry.Register(service)
	logs.Logf("services/registry: registered service=%s", service.Name())

	got, ok := registry.Get("svc-a")
	if !ok || got == nil {
		t.Fatalf("expected service svc-a to exist")
	}

	out, err := got.Actions()["ping"]()
	if err != nil {
		t.Fatalf("expected ping success, got %v", err)
	}
	if out != "pong" {
		t.Fatalf("expected pong output, got %q", out)
	}
	logs.Logf("services/registry: action ping returned=%q", out)

	snapshot := registry.All()
	delete(snapshot, "svc-a")
	if _, stillThere := registry.Get("svc-a"); !stillThere {
		t.Fatalf("expected registry to be unaffected by snapshot mutation")
	}
	logs.Logf("services/registry: snapshot mutation did not alter source map")
}

func TestAdminCommandsUsesRunnerAndReturnsOutput(t *testing.T) {
	runner := &fakeRunner{}
	admin := &AdminCommands{Runner: runner}

	status, err := admin.Status()
	if err != nil {
		t.Fatalf("status failed: %v", err)
	}
	logs.Logf("services/admin: status output=%v", status)

	actions := admin.Actions()
	for _, name := range []string{"net", "repo"} {
		act, ok := actions[name]
		if !ok {
			t.Fatalf("missing action %q", name)
		}
		out, err := act()
		if err != nil {
			t.Fatalf("action %q failed: %v", name, err)
		}
		logs.Logf("services/admin: action=%s output=%q", name, out)
	}

	if len(runner.calls) < 3 {
		t.Fatalf("expected runner calls, got %v", runner.calls)
	}
}
