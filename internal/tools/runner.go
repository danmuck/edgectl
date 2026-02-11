package tools

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"strings"
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

// EnvRunner wraps ExecRunner and prepends extra directories to PATH.
type EnvRunner struct {
	ExtraPaths []string
}

// Run executes a command with extra PATH directories prepended.
func (r EnvRunner) Run(name string, args ...string) ([]byte, []byte, int32, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if len(r.ExtraPaths) > 0 {
		env := os.Environ()
		pathPrefix := strings.Join(r.ExtraPaths, string(os.PathListSeparator))
		found := false
		for i, e := range env {
			if strings.HasPrefix(e, "PATH=") {
				env[i] = "PATH=" + pathPrefix + string(os.PathListSeparator) + e[5:]
				found = true
				break
			}
		}
		if !found {
			env = append(env, "PATH="+pathPrefix)
		}
		cmd.Env = env
	}

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
