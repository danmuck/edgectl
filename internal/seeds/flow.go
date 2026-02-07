package seeds

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

var ErrUnknownAction = errors.New("unknown seed action")

// FlowSeed is a deterministic seed used for local execution verification.
type FlowSeed struct{}

// NewFlowSeed creates the deterministic flow seed.
func NewFlowSeed() FlowSeed {
	return FlowSeed{}
}

func (s FlowSeed) Metadata() SeedMetadata {
	return SeedMetadata{
		ID:          "seed.flow",
		Name:        "Flow",
		Description: "Deterministic control-flow seed",
	}
}

func (s FlowSeed) Execute(action string, args map[string]string) (SeedResult, error) {
	switch action {
	case "status":
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte("flow status: ok\n"),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	case "echo":
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte(renderArgs(args)),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	case "step":
		name := strings.TrimSpace(args["name"])
		msg, code := deterministicStep(name)
		if code != 0 {
			return SeedResult{
				Status:   "error",
				Stdout:   nil,
				Stderr:   []byte(msg + "\n"),
				ExitCode: int32(code),
			}, ErrUnknownAction
		}
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte(msg + "\n"),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	default:
		errMsg := fmt.Sprintf("unknown action: %s", action)
		return SeedResult{
			Status:   "error",
			Stdout:   nil,
			Stderr:   []byte(errMsg + "\n"),
			ExitCode: 64,
		}, ErrUnknownAction
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
