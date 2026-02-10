package docker

import (
	"errors"
	"fmt"
	"strings"

	"github.com/danmuck/edgectl/internal/seeds"
	"github.com/danmuck/edgectl/internal/tools"
	logs "github.com/danmuck/smplog"
)

var (
	ErrUnknownAction = errors.New("unknown seed action")
	ErrCommandFailed = errors.New("seed command failed")
)

// Seed is a predefined service seed for controlling Docker container operations.
type Seed struct {
	runner tools.CommandRunner
}

// NewSeed constructs a docker seed with a local command runner.
func NewSeed() Seed {
	logs.Debug("seeds.docker.NewSeed")
	return NewSeedWithRunner(tools.ExecRunner{})
}

// NewSeedWithRunner constructs a docker seed with an explicit command runner.
func NewSeedWithRunner(runner tools.CommandRunner) Seed {
	if runner == nil {
		runner = tools.ExecRunner{}
	}
	return Seed{runner: runner}
}

// Metadata returns stable identity and capability description.
func (s Seed) Metadata() seeds.SeedMetadata {
	return seeds.SeedMetadata{
		ID:          "seed.docker",
		Name:        "Docker",
		Description: "Container runtime interface via docker CLI",
	}
}

// Operations returns docker control behavior catalog.
func (s Seed) Operations() []seeds.OperationSpec {
	return []seeds.OperationSpec{
		{Name: "status", Description: "check docker daemon status", Idempotent: true},
		{Name: "ps", Description: "list containers", Idempotent: true},
		{Name: "run", Description: "run a new container", Idempotent: false},
		{Name: "stop", Description: "stop a running container", Idempotent: true},
		{Name: "rm", Description: "remove a container", Idempotent: true},
		{Name: "logs", Description: "fetch container logs", Idempotent: true},
		{Name: "inspect", Description: "inspect a container", Idempotent: true},
	}
}

// Execute dispatches docker operations to CLI commands.
func (s Seed) Execute(action string, args map[string]string) (seeds.SeedResult, error) {
	act := strings.TrimSpace(action)
	container := argVal(args, "container")
	image := argVal(args, "image")
	flags := argVal(args, "flags")

	switch act {
	case "status":
		return s.exec("docker", "info")
	case "ps":
		cmdArgs := []string{"ps", "-a"}
		if flags != "" {
			cmdArgs = append(cmdArgs, strings.Fields(flags)...)
		}
		return s.exec("docker", cmdArgs...)
	case "run":
		if image == "" {
			return seeds.SeedResult{Status: "error", Stderr: []byte("missing image arg\n"), ExitCode: 64}, ErrUnknownAction
		}
		cmdArgs := []string{"run", "-d"}
		if flags != "" {
			cmdArgs = append(cmdArgs, strings.Fields(flags)...)
		}
		if container != "" {
			cmdArgs = append(cmdArgs, "--name", container)
		}
		cmdArgs = append(cmdArgs, image)
		return s.exec("docker", cmdArgs...)
	case "stop":
		if container == "" {
			return seeds.SeedResult{Status: "error", Stderr: []byte("missing container arg\n"), ExitCode: 64}, ErrUnknownAction
		}
		return s.exec("docker", "stop", container)
	case "rm":
		if container == "" {
			return seeds.SeedResult{Status: "error", Stderr: []byte("missing container arg\n"), ExitCode: 64}, ErrUnknownAction
		}
		return s.exec("docker", "rm", container)
	case "logs":
		if container == "" {
			return seeds.SeedResult{Status: "error", Stderr: []byte("missing container arg\n"), ExitCode: 64}, ErrUnknownAction
		}
		cmdArgs := []string{"logs"}
		if flags != "" {
			cmdArgs = append(cmdArgs, strings.Fields(flags)...)
		}
		cmdArgs = append(cmdArgs, container)
		return s.exec("docker", cmdArgs...)
	case "inspect":
		if container == "" {
			return seeds.SeedResult{Status: "error", Stderr: []byte("missing container arg\n"), ExitCode: 64}, ErrUnknownAction
		}
		return s.exec("docker", "inspect", container)
	default:
		errMsg := fmt.Sprintf("unknown action: %s", act)
		return seeds.SeedResult{Status: "error", Stderr: []byte(errMsg + "\n"), ExitCode: 64}, ErrUnknownAction
	}
}

// exec runs a CLI command and maps the result to a SeedResult.
func (s Seed) exec(name string, args ...string) (seeds.SeedResult, error) {
	stdout, stderr, exitCode, err := s.runner.Run(name, args...)
	if err != nil {
		if len(stderr) == 0 {
			stderr = []byte(err.Error() + "\n")
		}
		if exitCode == 0 {
			exitCode = 1
		}
		return seeds.SeedResult{
			Status:   "error",
			Stdout:   stdout,
			Stderr:   stderr,
			ExitCode: exitCode,
		}, fmt.Errorf("%w: %v", ErrCommandFailed, err)
	}
	return seeds.SeedResult{Status: "ok", Stdout: stdout, Stderr: stderr, ExitCode: 0}, nil
}

// argVal extracts a trimmed arg value from the args map.
func argVal(args map[string]string, key string) string {
	if args == nil {
		return ""
	}
	return strings.TrimSpace(args[key])
}
