package tools

import (
	"bytes"
	"errors"
	"os/exec"
)

// CommandRunner abstracts shell command execution for runtime adapters.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, []byte, int32, error)
}

// ExecRunner executes commands on the local host.
type ExecRunner struct{}

// tools command-runner implementation backed by os/exec.
func (r ExecRunner) Run(name string, args ...string) ([]byte, []byte, int32, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), stderr.Bytes(), int32(exitErr.ExitCode()), err
	}

	exitCode := int32(1)
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		exitCode = 127
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}
