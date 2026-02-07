package testlog

import (
	"testing"

	"github.com/danmuck/edgectl/internal/logging"
	logs "github.com/danmuck/smplog"
)

func Start(t *testing.T) {
	t.Helper()
	logging.ConfigureTests()
	logs.Infof("test=%s", t.Name())
}
