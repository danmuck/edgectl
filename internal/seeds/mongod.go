package seeds

import (
	"errors"
	"fmt"
	"strings"

	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

const (
	DefaultMongodUnit = "mongod"
)

var ErrCommandFailed = errors.New("seed command failed")

// MongodSeed is a predefined service seed for controlling mongod service operations.
type MongodSeed struct {
	unit   string
	runner tools.CommandRunner
}

func NewMongodSeed() MongodSeed {
	logs.Debug("seeds.NewMongodSeed")
	return NewMongodSeedWithRunner(DefaultMongodUnit, tools.ExecRunner{})
}

func NewMongodSeedWithRunner(unit string, runner tools.CommandRunner) MongodSeed {
	resolvedUnit := strings.TrimSpace(unit)
	if resolvedUnit == "" {
		resolvedUnit = DefaultMongodUnit
	}
	if runner == nil {
		runner = tools.ExecRunner{}
	}
	return MongodSeed{
		unit:   resolvedUnit,
		runner: runner,
	}
}

func (s MongodSeed) Metadata() SeedMetadata {
	return SeedMetadata{
		ID:          "seed.mongod",
		Name:        "MongoDB (mongod)",
		Description: "Predefined mongod service adapter for Linux hosts",
	}
}

func (s MongodSeed) Operations() []OperationSpec {
	return []OperationSpec{
		{Name: "status", Description: "read mongod service status", Idempotent: true},
		{Name: "start", Description: "start mongod service", Idempotent: true},
		{Name: "stop", Description: "stop mongod service", Idempotent: true},
		{Name: "restart", Description: "restart mongod service", Idempotent: false},
		{Name: "version", Description: "read mongod binary version", Idempotent: true},
	}
}

func (s MongodSeed) Execute(action string, args map[string]string) (SeedResult, error) {
	act := strings.TrimSpace(action)
	unit := s.unit
	if args != nil {
		if override := strings.TrimSpace(args["unit"]); override != "" {
			unit = override
		}
	}

	logs.Debugf("seeds.MongodSeed.Execute action=%q unit=%q", act, unit)
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
		logs.Warnf("seeds.MongodSeed.Execute unknown action=%q", act)
		return SeedResult{
			Status:   "error",
			Stdout:   nil,
			Stderr:   []byte(errMsg + "\n"),
			ExitCode: 64,
		}, ErrUnknownAction
	}
}

func (s MongodSeed) exec(name string, args ...string) (SeedResult, error) {
	stdout, stderr, exitCode, err := s.runner.Run(name, args...)
	if err != nil {
		logs.Errf("seeds.MongodSeed.exec command failed cmd=%s args=%v exit=%d err=%v", name, args, exitCode, err)
		if len(stderr) == 0 {
			stderr = []byte(err.Error() + "\n")
		}
		if exitCode == 0 {
			exitCode = 1
		}
		return SeedResult{
			Status:   "error",
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
		}, fmt.Errorf("%w: %v", ErrCommandFailed, err)
	}

	logs.Infof("seeds.MongodSeed.exec ok cmd=%s args=%v", name, args)
	return SeedResult{
		Status:   "ok",
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: 0,
	}, nil
}
