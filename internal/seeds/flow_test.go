package seeds

import (
	"bytes"
	"errors"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestFlowSeedMetadata(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	meta := seed.Metadata()
	if meta.ID != "seed.flow" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}
}

func TestFlowSeedStatusDeterministic(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	res, err := seed.Execute("status", nil)
	if err != nil {
		t.Fatalf("status execute: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected status result: %+v", res)
	}
	if !bytes.Equal(res.Stdout, []byte("flow status: ok\n")) {
		t.Fatalf("unexpected stdout: %q", string(res.Stdout))
	}
}

func TestFlowSeedEchoArgOrderIndependent(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	resA, err := seed.Execute("echo", map[string]string{"b": "2", "a": "1"})
	if err != nil {
		t.Fatalf("echo execute A: %v", err)
	}
	resB, err := seed.Execute("echo", map[string]string{"a": "1", "b": "2"})
	if err != nil {
		t.Fatalf("echo execute B: %v", err)
	}
	if !bytes.Equal(resA.Stdout, resB.Stdout) {
		t.Fatalf("echo output should be deterministic: %q vs %q", string(resA.Stdout), string(resB.Stdout))
	}
	if !bytes.Equal(resA.Stdout, []byte("flow echo: a=1,b=2\n")) {
		t.Fatalf("unexpected canonical echo output: %q", string(resA.Stdout))
	}
}

func TestFlowSeedStepDeterministic(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	res, err := seed.Execute("step", map[string]string{"name": "plan"})
	if err != nil {
		t.Fatalf("step execute: %v", err)
	}
	if res.Status != "ok" || res.ExitCode != 0 {
		t.Fatalf("unexpected step result: %+v", res)
	}
	if !bytes.Equal(res.Stdout, []byte("flow step: plan -> queued\n")) {
		t.Fatalf("unexpected step stdout: %q", string(res.Stdout))
	}
}

func TestFlowSeedStepUnknownDeterministicFailure(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	res, err := seed.Execute("step", map[string]string{"name": "unknown"})
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
	if res.Status != "error" || res.ExitCode != 2 {
		t.Fatalf("unexpected step error result: %+v", res)
	}
	if !bytes.Equal(res.Stderr, []byte("flow step: unknown\n")) {
		t.Fatalf("unexpected step stderr: %q", string(res.Stderr))
	}
}

func TestFlowSeedUnknownActionDeterministicFailure(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	res, err := seed.Execute("nope", nil)
	if !errors.Is(err, ErrUnknownAction) {
		t.Fatalf("expected ErrUnknownAction, got %v", err)
	}
	if res.Status != "error" || res.ExitCode == 0 {
		t.Fatalf("unexpected error result: %+v", res)
	}
	if !bytes.Equal(res.Stderr, []byte("unknown action: nope\n")) {
		t.Fatalf("unexpected stderr: %q", string(res.Stderr))
	}
}

func TestFlowSeedEchoEmptyArgsDeterministic(t *testing.T) {
	testlog.Start(t)
	seed := NewFlowSeed()
	res, err := seed.Execute("echo", nil)
	if err != nil {
		t.Fatalf("echo execute: %v", err)
	}
	if !bytes.Equal(res.Stdout, []byte("flow echo: {}\n")) {
		t.Fatalf("unexpected empty-echo stdout: %q", string(res.Stdout))
	}
}

func TestFlowSeedLocalRegistryInvoke(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	seed := NewFlowSeed()
	if err := r.Register(seed); err != nil {
		t.Fatalf("register flow seed: %v", err)
	}
	resolved, ok := r.Resolve("seed.flow")
	if !ok {
		t.Fatalf("seed.flow not found")
	}
	res, err := resolved.Execute("status", nil)
	if err != nil {
		t.Fatalf("invoke status: %v", err)
	}
	if res.Status != "ok" || !bytes.Equal(res.Stdout, []byte("flow status: ok\n")) {
		t.Fatalf("unexpected invoke result: %+v", res)
	}
}
