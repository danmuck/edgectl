package seeds

import (
	"fmt"

	"github.com/danmuck/edgectl/internal/tools"
)

// ExecCommand runs a CLI command through a runner and maps the result to a SeedResult.
// Returns a wrapped error using the provided sentinel when the command fails.
func ExecCommand(runner tools.CommandRunner, sentinel error, name string, args ...string) (SeedResult, error) {
	stdout, stderr, exitCode, err := runner.Run(name, args...)
	if err != nil {
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
		}, fmt.Errorf("%w: %v", sentinel, err)
	}
	return SeedResult{Status: "ok", Stdout: stdout, Stderr: stderr, ExitCode: 0}, nil
}
