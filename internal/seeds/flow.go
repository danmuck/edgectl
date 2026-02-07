package seeds

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	logs "github.com/danmuck/smplog"
)

var ErrUnknownAction = errors.New("unknown seed action")

// Seeds package deterministic seed used for local execution verification.
type FlowSeed struct{}

// Seeds package constructor for deterministic flow seed.
func NewFlowSeed() FlowSeed {
	logs.Debug("seeds.NewFlowSeed")
	return FlowSeed{}
}

// Seeds package metadata provider for stable identity and capability description.
func (s FlowSeed) Metadata() SeedMetadata {
	logs.Debug("seeds.FlowSeed.Metadata")
	return SeedMetadata{
		ID:          "seed.flow",
		Name:        "Flow",
		Description: "Deterministic control-flow seed",
	}
}

// Seeds package operation catalog for deterministic flow behavior.
func (s FlowSeed) Operations() []OperationSpec {
	return []OperationSpec{
		{Name: "status", Description: "deterministic health/status response", Idempotent: true},
		{Name: "echo", Description: "deterministic argument echo", Idempotent: true},
		{Name: "step", Description: "deterministic pseudo-step mapping", Idempotent: true},
	}
}

// Seeds package deterministic operation dispatcher.
func (s FlowSeed) Execute(action string, args map[string]string) (SeedResult, error) {
	logs.Debugf("seeds.FlowSeed.Execute action=%q args=%d", action, len(args))
	switch action {
	case "status":
		logs.Infof("seeds.FlowSeed.Execute status")
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte("flow status: ok\n"),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	case "echo":
		logs.Infof("seeds.FlowSeed.Execute echo")
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte(renderArgs(args)),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	case "step":
		name := strings.TrimSpace(args["name"])
		logs.Infof("seeds.FlowSeed.Execute step name=%q", name)
		msg, code := deterministicStep(name)
		if code != 0 {
			logs.Warnf("seeds.FlowSeed.Execute step failed name=%q code=%d", name, code)
			return SeedResult{
				Status:   "error",
				Stdout:   nil,
				Stderr:   []byte(msg + "\n"),
				ExitCode: int32(code),
			}, ErrUnknownAction
		}
		logs.Infof("seeds.FlowSeed.Execute step ok name=%q", name)
		return SeedResult{
			Status:   "ok",
			Stdout:   []byte(msg + "\n"),
			Stderr:   nil,
			ExitCode: 0,
		}, nil
	default:
		errMsg := fmt.Sprintf("unknown action: %s", action)
		logs.Warnf("seeds.FlowSeed.Execute unknown action=%q", action)
		return SeedResult{
			Status:   "error",
			Stdout:   nil,
			Stderr:   []byte(errMsg + "\n"),
			ExitCode: 64,
		}, ErrUnknownAction
	}
}

// Seeds package helper rendering args in deterministic key order for echo output.
func renderArgs(args map[string]string) string {
	logs.Debugf("seeds.renderArgs args=%d", len(args))
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

// Seeds package helper mapping step names to deterministic outputs and exit codes.
func deterministicStep(name string) (string, int) {
	logs.Debugf("seeds.deterministicStep name=%q", name)
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
