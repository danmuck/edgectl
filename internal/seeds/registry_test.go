package seeds

import (
	"errors"
	"reflect"
	"testing"

	"github.com/danmuck/edgectl/internal/testutil/testlog"
)

type fakeSeed struct {
	meta SeedMetadata
}

func (f fakeSeed) Metadata() SeedMetadata {
	return f.meta
}

func (f fakeSeed) Operations() []OperationSpec {
	return []OperationSpec{{Name: "status", Description: "fake status", Idempotent: true}}
}

func (f fakeSeed) Execute(action string, args map[string]string) (SeedResult, error) {
	return SeedResult{Status: "ok", ExitCode: 0}, nil
}

func TestRegisterResolveAndDuplicate(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	s := fakeSeed{meta: SeedMetadata{ID: "seed.flow", Name: "Flow", Description: "Deterministic flow seed"}}

	if err := r.Register(s); err != nil {
		t.Fatalf("register: %v", err)
	}
	if err := r.Register(s); !errors.Is(err, ErrSeedExists) {
		t.Fatalf("expected ErrSeedExists, got %v", err)
	}
	got, ok := r.Resolve("seed.flow")
	if !ok || got.Metadata().ID != "seed.flow" {
		t.Fatalf("resolve failed: ok=%v id=%q", ok, got.Metadata().ID)
	}
}

func TestResolveMissingSeed(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	_, ok := r.Resolve("seed.missing")
	if ok {
		t.Fatalf("expected missing seed to return ok=false")
	}
}

func TestListMetadataSorted(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	_ = r.Register(fakeSeed{meta: SeedMetadata{ID: "seed.z", Name: "Z", Description: "z"}})
	_ = r.Register(fakeSeed{meta: SeedMetadata{ID: "seed.a", Name: "A", Description: "a"}})
	_ = r.Register(fakeSeed{meta: SeedMetadata{ID: "seed.m", Name: "M", Description: "m"}})

	list := r.ListMetadata()
	ids := []string{list[0].ID, list[1].ID, list[2].ID}
	want := []string{"seed.a", "seed.m", "seed.z"}
	if !reflect.DeepEqual(ids, want) {
		t.Fatalf("metadata not sorted: got=%v want=%v", ids, want)
	}
}

func TestValidateMetadataFailures(t *testing.T) {
	testlog.Start(t)
	cases := []SeedMetadata{
		{ID: "", Name: "Flow", Description: "x"},
		{ID: "seed.flow", Name: "", Description: "x"},
		{ID: "seed.flow", Name: "Flow", Description: ""},
		{ID: "Seed.Flow", Name: "Flow", Description: "x"},
		{ID: ".seed.flow", Name: "Flow", Description: "x"},
		{ID: "seed..flow", Name: "Flow", Description: "x"},
	}
	for _, meta := range cases {
		if err := ValidateMetadata(meta); !errors.Is(err, ErrInvalidMetadata) {
			t.Fatalf("expected ErrInvalidMetadata for meta=%+v, got %v", meta, err)
		}
	}
}

func TestRegisterNilSeed(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	if err := r.Register(nil); !errors.Is(err, ErrSeedNil) {
		t.Fatalf("expected ErrSeedNil, got %v", err)
	}
}

func TestRegisterInvalidMetadataFails(t *testing.T) {
	testlog.Start(t)
	r := NewRegistry()
	s := fakeSeed{meta: SeedMetadata{ID: "Seed.Invalid", Name: "Bad", Description: "bad id format"}}
	if err := r.Register(s); !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("expected ErrInvalidMetadata, got %v", err)
	}
}
