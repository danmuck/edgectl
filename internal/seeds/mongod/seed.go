package mongod

import (
	"errors"
	"fmt"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

const (
	DefaultUnit = "mongod"
)

var (
	ErrUnknownAction = errors.New("unknown seed action")
	ErrCommandFailed = errors.New("seed command failed")
)

// Seed is a predefined service seed for controlling mongod operations.
type Seed struct {
	unit   string
	runner tools.CommandRunner
}

// NewSeed constructs a mongod seed with default unit and local runner.
func NewSeed() Seed {
	logs.Debug("seeds.mongod.NewSeed")
	return NewSeedWithRunner(DefaultUnit, tools.ExecRunner{})
}

// NewSeedWithRunner constructs a mongod seed with explicit unit and runner.
func NewSeedWithRunner(unit string, runner tools.CommandRunner) Seed {
	resolvedUnit := strings.TrimSpace(unit)
	if resolvedUnit == "" {
		resolvedUnit = DefaultUnit
	}
	if runner == nil {
		runner = tools.ExecRunner{}
	}
	return Seed{unit: resolvedUnit, runner: runner}
}

// Metadata returns stable identity and capability description.
func (s Seed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{
		ID:          "seed.mongod",
		Name:        "MongoDB (mongod)",
		Description: "Predefined mongod service adapter for Linux hosts",
	}
}

// Operations returns mongod control behavior catalog.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "status", Description: "read mongod service status", Idempotent: true},
		{Name: "start", Description: "start mongod service", Idempotent: true},
		{Name: "stop", Description: "stop mongod service", Idempotent: true},
		{Name: "restart", Description: "restart mongod service", Idempotent: false},
		{Name: "version", Description: "read mongod binary version", Idempotent: true},
	}
}

// CommandCatalog returns guided operator-facing command templates for this seed.
func (s Seed) CommandCatalog() []seeds.CommandTemplate {
	seedID := s.Metadata().ID
	unitArg := []seeds.CommandArgSpec{
		{Key: "unit", Prompt: "systemd unit", Required: false, DefaultValue: "mongod"},
	}
	return []seeds.CommandTemplate{
		{
			ID:           seedID + ".status",
			Label:        "MongoDB Status",
			Description:  "Read mongod service status.",
			SeedSelector: seedID,
			Operation:    "status",
			Args:         unitArg,
		},
		{
			ID:           seedID + ".start",
			Label:        "MongoDB Start",
			Description:  "Start mongod service.",
			SeedSelector: seedID,
			Operation:    "start",
			Args:         unitArg,
		},
		{
			ID:           seedID + ".stop",
			Label:        "MongoDB Stop",
			Description:  "Stop mongod service.",
			SeedSelector: seedID,
			Operation:    "stop",
			Args:         unitArg,
		},
		{
			ID:           seedID + ".restart",
			Label:        "MongoDB Restart",
			Description:  "Restart mongod service.",
			SeedSelector: seedID,
			Operation:    "restart",
			Args:         unitArg,
		},
		{
			ID:           seedID + ".version",
			Label:        "MongoDB Version",
			Description:  "Read mongod binary version.",
			SeedSelector: seedID,
			Operation:    "version",
		},
	}
}

// Execute dispatches mongod operations to system commands.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	act := strings.TrimSpace(action)
	unit := s.unit
	if args != nil {
		if override := strings.TrimSpace(args["unit"]); override != "" {
			unit = override
		}
	}
	switch act {
	case "status":
		return s.exec("systemctl", "is-active", unit)
	case "start":
		return s.exec("systemctl", "start", unit)
	case "stop":
		return s.exec("systemctl", "stop", unit)
	case "restart":
		return s.exec("systemctl", "restart", unit)
	case "version":
		return s.exec("mongod", "--version")
	default:
		errMsg := fmt.Sprintf("unknown action: %s", act)
		return seeds.SeedResult{Status: "error", Stderr: []byte(errMsg + "\n"), ExitCode: 64}, ErrUnknownAction
	}
}

func (s Seed) exec(name string, args ...string) (seeds.SeedResult, error) {
	return seeds.ExecCommand(s.runner, ErrCommandFailed, name, args...)
}
