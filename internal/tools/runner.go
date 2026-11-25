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
	Run(name string, args ...string) ([]byte, []byte, uint32, error)
}

// ExecRunner executes commands on the local host.
type ExecRunner struct{}

// tools command-runner implementation backed by os/exec.
func (r ExecRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
	cmd := exec.Command(name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	return runCmd(cmd, &stdout, &stderr)
}

// EnvRunner wraps ExecRunner and prepends extra directories to PATH.
type EnvRunner struct {
	ExtraPaths []string
}

// Run executes a command with extra PATH directories prepended.
func (r EnvRunner) Run(name string, args ...string) ([]byte, []byte, uint32, error) {
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

	return runCmd(cmd, &stdout, &stderr)
}

// runCmd executes a prepared command and extracts exit code from the result.
func runCmd(cmd *exec.Cmd, stdout *bytes.Buffer, stderr *bytes.Buffer) ([]byte, []byte, uint32, error) {
	err := cmd.Run()
	if err == nil {
		return stdout.Bytes(), stderr.Bytes(), 0, nil
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := exitErr.ExitCode()
		if code < 0 {
			// Negative exit code means the process was killed by a signal.
			// Normalize to 1 to avoid bogus uint32 overflow values.
			code = 1
		}
		return stdout.Bytes(), stderr.Bytes(), uint32(code), err
	}

	exitCode := uint32(1)
	var execErr *exec.Error
	if errors.As(err, &execErr) {
		exitCode = 127
	}
	return stdout.Bytes(), stderr.Bytes(), exitCode, err
}
