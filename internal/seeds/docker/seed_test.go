package docker

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeRunner struct {
	stdout   []byte
	stderr   []byte
	exitCode int32
	err      error
	name     string
	args     []string
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, []byte, int32, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return r.stdout, r.stderr, r.exitCode, r.err
}

func TestSeedMetadata(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(&fakeRunner{})
	meta := seed.Metadata()
	if meta.ID != "seed.docker" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := seeds.ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}
}

func TestSeedStatusUsesDockerInfo(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("Docker info output\n")}
	seed := NewSeedWithRunner(r)
	res, err := seed.Execute("status", nil)
	if err != nil {
		t.Fatalf("status execute failed: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected status result: %+v", res)
	}
	if r.name != "docker" || len(r.args) != 1 || r.args[0] != "info" {
		t.Fatalf("unexpected command: name=%q args=%v", r.name, r.args)
	}
}

func TestSeedPsListsContainers(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("CONTAINER ID\n")}
	seed := NewSeedWithRunner(r)
	res, err := seed.Execute("ps", nil)
	if err != nil {
		t.Fatalf("ps execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected ps result: %+v", res)
	}
	if r.name != "docker" || len(r.args) < 2 || r.args[0] != "ps" || r.args[1] != "-a" {
		t.Fatalf("unexpected command: name=%q args=%v", r.name, r.args)
	}
}

func TestSeedRunRequiresImage(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(&fakeRunner{})
	res, err := seed.Execute("run", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected error for missing image, got %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("expected error status: %+v", res)
	}
}

func TestSeedRunWithImage(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("abc123\n")}
	seed := NewSeedWithRunner(r)
	res, err := seed.Execute("run", map[string]string{"image": "nginx:latest", "container": "web"})
	if err != nil {
		t.Fatalf("run execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected run result: %+v", res)
	}
	if r.name != "docker" {
		t.Fatalf("expected docker command, got %q", r.name)
	}
	// Expect: run -d --name web nginx:latest
	found := false
	for _, a := range r.args {
		if a == "nginx:latest" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected image in args: %v", r.args)
	}
}

func TestSeedStopRequiresContainer(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(&fakeRunner{})
	res, err := seed.Execute("stop", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected error for missing container, got %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("expected error status: %+v", res)
	}
}

func TestSeedStopContainer(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("mycontainer\n")}
	seed := NewSeedWithRunner(r)
	res, err := seed.Execute("stop", map[string]string{"container": "mycontainer"})
	if err != nil {
		t.Fatalf("stop execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected stop result: %+v", res)
	}
	if r.name != "docker" || len(r.args) != 2 || r.args[0] != "stop" || r.args[1] != "mycontainer" {
		t.Fatalf("unexpected command: name=%q args=%v", r.name, r.args)
	}
}

func TestSeedCommandFailure(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stderr: []byte("docker failed\n"), exitCode: 1, err: errors.New("exit status 1")}
	seed := NewSeedWithRunner(r)
	res, err := seed.Execute("status", nil)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("expected ErrCommandFailed, got %v", err)
	}
	if res.Status != "error" || res.ExitCode != 1 {
		t.Fatalf("unexpected failure result: %+v", res)
	}
}

func TestSeedUnknownAction(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(&fakeRunner{})
	res, err := seed.Execute("bogus", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
	if res.Status != "error" {
		t.Fatalf("expected error status: %+v", res)
	}
}
