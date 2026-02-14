package mongod

import (
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeRunner struct {
	stdout   []byte
	stderr   []byte
	exitCode uint32
	err      error
	name     string
	args     []string
}

func (r *fakeRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
	r.name = name
	r.args = append([]string{}, args...)
	return r.stdout, r.stderr, r.exitCode, r.err
}

func TestSeedMetadata(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(DefaultUnit, &fakeRunner{})
	meta := seed.Metadata()
	if meta.ID != "seed.mongod" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := seeds.ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}
}

func TestSeedStatusUsesSystemctl(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stdout: []byte("active\n")}
	seed := NewSeedWithRunner("mongod", r)
	res, err := seed.Execute("status", nil)
	if err != nil {
		t.Fatalf("status execute failed: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected status result: %+v", res)
	}
	if r.name != "systemctl" || len(r.args) != 2 || r.args[0] != "is-active" || r.args[1] != "mongod" {
		t.Fatalf("unexpected command: name=%q args=%v", r.name, r.args)
	}
}

func TestSeedCommandFailure(t *testing.T) {
	testlog.Start(t)
	r := &fakeRunner{stderr: []byte("systemctl failed\n"), exitCode: 5, err: errors.New("exit status 5")}
	seed := NewSeedWithRunner("mongod", r)
	res, err := seed.Execute("start", nil)
	if !errors.Is(err, ErrCommandFailed) {
		t.Fatalf("expected ErrCommandFailed, got %v", err)
	}
	if res.Status != "error" || res.ExitCode != 5 {
		t.Fatalf("unexpected failure result: %+v", res)
	}
}

func TestSeedCommandCatalog(t *testing.T) {
	testlog.Start(t)
	seed := NewSeedWithRunner(DefaultUnit, &fakeRunner{})
	catalog := seed.CommandCatalog()
	if len(catalog) != 5 {
		t.Fatalf("unexpected catalog size: %d", len(catalog))
	}
	if catalog[0].SeedSelector != "seed.mongod" || catalog[0].Operation != "status" {
		t.Fatalf("unexpected first catalog entry: %+v", catalog[0])
	}
	if len(catalog[0].Args) != 1 || catalog[0].Args[0].Key != "unit" {
		t.Fatalf("unexpected unit arg spec: %+v", catalog[0].Args)
	}
}
