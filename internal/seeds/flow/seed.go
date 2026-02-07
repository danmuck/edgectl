package flow

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
	logs "github.com/danmuck/smplog"
)

var errUnknownAction = errors.New("unknown seed action")

// Seed is a deterministic seed used for local execution verification.
type Seed struct{}

// NewSeed constructs a deterministic flow seed.
func NewSeed() Seed {
	logs.Debug("seeds.flow.NewSeed")
	return Seed{}
}

// Metadata returns stable identity and capability description.
func (s Seed) Metadata() seeds.SeedMetadata {
	logs.Debug("seeds.flow.Seed.Metadata")
	return seeds.SeedMetadata{
		ID:          "seed.flow",
		Name:        "Flow",
		Description: "Deterministic control-flow seed",
	}
}

// Operations returns deterministic flow behavior catalog.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "status", Description: "deterministic health/status response", Idempotent: true},
		{Name: "echo", Description: "deterministic argument echo", Idempotent: true},
		{Name: "step", Description: "deterministic pseudo-step mapping", Idempotent: true},
	}
}

// Execute dispatches deterministic flow operations.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	logs.Debugf("seeds.flow.Seed.Execute action=%q args=%d", action, len(args))
	switch action {
	case "status":
		return seeds.SeedResult{Status: "ok", Stdout: []byte("flow status: ok\n"), ExitCode: 0}, nil
	case "echo":
		return seeds.SeedResult{Status: "ok", Stdout: []byte(renderArgs(args)), ExitCode: 0}, nil
	case "step":
		name := strings.TrimSpace(args["name"])
		msg, code := deterministicStep(name)
		if code != 0 {
			return seeds.SeedResult{Status: "error", Stderr: []byte(msg + "\n"), ExitCode: int32(code)}, errUnknownAction
		}
		return seeds.SeedResult{Status: "ok", Stdout: []byte(msg + "\n"), ExitCode: 0}, nil
	default:
		errMsg := fmt.Sprintf("unknown action: %s", action)
		return seeds.SeedResult{Status: "error", Stderr: []byte(errMsg + "\n"), ExitCode: 64}, errUnknownAction
	}
}

func renderArgs(args map[string]string) string {
	if len(args) == 0 {
		return "flow echo: {}\n"
	}
	keys := make([]string, 0, len(args))
	for k := range args {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, k := range keys {
		pairs = append(pairs, fmt.Sprintf("%s=%s", k, args[k]))
	}
	return "flow echo: " + strings.Join(pairs, ",") + "\n"
}

func deterministicStep(name string) (string, int) {
	switch name {
	case "init":
		return "flow step: init -> ready", 0
	case "plan":
		return "flow step: plan -> queued", 0
	case "apply":
		return "flow step: apply -> complete", 0
	default:
		return "flow step: unknown", 2
	}
}
