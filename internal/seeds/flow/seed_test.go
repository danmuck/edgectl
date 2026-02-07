package flow

import (
	"bytes"
	"testing"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

func TestSeedMetadata(t *testing.T) {
	testlog.Start(t)
	seed := NewSeed()
	meta := seed.Metadata()
	if meta.ID != "seed.flow" {
		t.Fatalf("unexpected id: %q", meta.ID)
	}
	if err := seeds.ValidateMetadata(meta); err != nil {
		t.Fatalf("metadata should be valid: %v", err)
	}
}

func TestSeedStatusDeterministic(t *testing.T) {
	testlog.Start(t)
	seed := NewSeed()
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
