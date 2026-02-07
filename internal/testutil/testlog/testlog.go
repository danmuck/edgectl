package testlog

import (
	"testing"

	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

// testlog helper that configures test logging and tags each test name.
func Start(t *testing.T) {
	t.Helper()
	logging.ConfigureTests()
	logs.Infof("test=%s", t.Name())
}
