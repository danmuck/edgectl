package seeds

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeRunnerResult struct {
	stdout   []byte
	stderr   []byte
	exitCode int32
	err      error
}

type fakeRunner struct {
	result fakeRunnerResult
	last   struct {
		name string
		args []string
	}
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, []byte, int32, error) {
	r.last.name = name
	r.last.args = append([]string(nil), args...)
	return r.result.stdout, r.result.stderr, r.result.exitCode, r.result.err
}

func TestMongodSeedMetadataAndOperations(t *testing.T) {
	testlog.Start(t)
	seed := NewMongodSeedWithRunner(DefaultMongodUnit, &fakeRunner{})

	meta := seed.Metadata()
	if meta.ID != "seed.mongod" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}

	ops := seed.Operations()
	if len(ops) == 0 {
		t.Fatalf("operations should not be empty")
	}
}

func TestMongodSeedStatusUsesSystemctl(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{
		result: fakeRunnerResult{
			stdout:   []byte("active\n"),
			stderr:   nil,
			exitCode: 0,
			err:      nil,
		},
	}
	seed := NewMongodSeedWithRunner("mongod", r)
	res, err := seed.Execute("status", nil)
	if err != nil {
		t.Fatalf("status execute failed: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected status result: %+v", res)
	}
	if r.last.name != "systemctl" || len(r.last.args) != 2 || r.last.args[0] != "is-active" || r.last.args[1] != "mongod" {
		t.Fatalf("unexpected command: name=%q args=%v", r.last.name, r.last.args)
	}
}

func TestMongodSeedRestartWithUnitOverride(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{
		result: fakeRunnerResult{
			stdout:   nil,
			stderr:   nil,
			exitCode: 0,
			err:      nil,
		},
	}
	seed := NewMongodSeedWithRunner("mongod", r)
	_, err := seed.Execute("restart", map[string]string{"unit": "mongodb"})
	if err != nil {
		t.Fatalf("restart execute failed: %v", err)
	}
	if r.last.name != "systemctl" || len(r.last.args) != 2 || r.last.args[0] != "restart" || r.last.args[1] != "mongodb" {
		t.Fatalf("unexpected command: name=%q args=%v", r.last.name, r.last.args)
	}
}

func TestMongodSeedVersionCommand(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{
		result: fakeRunnerResult{
			stdout:   []byte("db version v7\n"),
			stderr:   nil,
			exitCode: 0,
			err:      nil,
		},
	}
	seed := NewMongodSeedWithRunner("mongod", r)
	res, err := seed.Execute("version", nil)
	if err != nil {
		t.Fatalf("version execute failed: %v", err)
	}
	if res.Status != "ok" {
		t.Fatalf("unexpected status: %q", res.Status)
	}
	if r.last.name != "mongod" || len(r.last.args) != 1 || r.last.args[0] != "--version" {
		t.Fatalf("unexpected command: name=%q args=%v", r.last.name, r.last.args)
	}
}

func TestMongodSeedCommandFailure(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{
		result: fakeRunnerResult{
			stdout:   []byte(""),
			stderr:   []byte("systemctl: failed\n"),
			exitCode: 5,
			err:      errors.New("exit status 5"),
		},
	}
	seed := NewMongodSeedWithRunner("mongod", r)
	res, err := seed.Execute("start", nil)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("expected ErrCommandFailed, got %v", err)
	}
	if res.Status != "error" || res.ExitCode != 5 {
		t.Fatalf("unexpected failure result: %+v", res)
	}
}

func TestMongodSeedUnknownAction(t *testing.T) {
	testlog.Start(t)
	seed := NewMongodSeedWithRunner("mongod", &fakeRunner{})
	res, err := seed.Execute("invalid", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
	if res.Status != "error" || res.ExitCode == 0 {
		t.Fatalf("unexpected result: %+v", res)
	}
}
