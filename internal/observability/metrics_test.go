package observability

import (
	"testing"
	"time"

	logs "github.com/danmuck/smplog"
)

func TestRegisterMetricsAndRecordersAreSafe(t *testing.T) {
	RegisterMetrics()
	RegisterMetrics()

	RecordHTTPRequest("mirage-a", "GET", "/health", 200, 12*time.Millisecond)
	RecordGhostProxy("mirage-a", "ghost-a", "POST", "/seeds/flow/actions/intent", 200, 24*time.Millisecond, true)

	logs.Logf("observability/metrics: registration idempotent and recording paths executed")
}
