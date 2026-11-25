package adminauth

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/danmuck/edgectl/internal/identity"
	logs "github.com/danmuck/smplog"
)

// Writer that always fails, standing in for a closed or full log destination.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("sink closed") }

// Installs a logger at the given level writing to buf and restores the previous config on cleanup.
func configureLogs(t *testing.T, level logs.Level, buf *bytes.Buffer) {
	t.Helper()
	prev := logs.Configured()
	cfg := prev
	cfg.Writer = buf
	cfg.Level = level
	cfg.Bypass = true
	logs.Configure(cfg)
	t.Cleanup(func() { logs.Configure(prev) })
}

// admin_security.toml [audit]: allow and deny decisions are recorded even when the configured
// log level would drop info and warn lines.
func TestAuditIsWrittenWhenLogLevelSuppressesIt(t *testing.T) {
	for _, level := range []logs.Level{logs.ErrorLevel, logs.Disabled} {
		var buf bytes.Buffer
		configureLogs(t, level, &buf)
		peer := Peer{Principal: identity.Principal{Role: identity.RoleOperator, ID: "ops"}, Authenticated: true, Addr: "127.0.0.1:1"}
		Audit("mirage", peer, "submit_issue", DecisionAllow, 0)
		Audit("mirage", Peer{Addr: "127.0.0.1:2"}, "deploy_ghost", DecisionDeny, 1300)

		lines := strings.Split(strings.TrimSpace(buf.String()), "\n")
		if len(lines) != 2 {
			t.Fatalf("level=%v: expected 2 audit lines, got %d: %q", level, len(lines), buf.String())
		}
		for _, want := range []string{"AUDIT admin.audit", "component=mirage", `principal="edgectl://operator/ops"`, "role=operator", `action="submit_issue"`, "decision=allow", "error_code=0", "timestamp_ms="} {
			if !strings.Contains(lines[0], want) {
				t.Errorf("level=%v: allow line missing %q: %s", level, want, lines[0])
			}
		}
		for _, want := range []string{`principal="unauthenticated"`, "role=none", "decision=deny", "error_code=1300"} {
			if !strings.Contains(lines[1], want) {
				t.Errorf("level=%v: deny line missing %q: %s", level, want, lines[1])
			}
		}
	}
}

// At a level that passes the record, it goes through the normal logger exactly once.
func TestAuditUsesNormalLoggerWhenLevelPermits(t *testing.T) {
	var buf bytes.Buffer
	configureLogs(t, logs.InfoLevel, &buf)
	if err := Record(AuditInfo, "admin.audit probe=1"); err != nil {
		t.Fatalf("record: %v", err)
	}
	out := buf.String()
	if strings.Count(out, "admin.audit probe=1") != 1 || strings.Contains(out, "AUDIT ") {
		t.Fatalf("expected one logger-emitted line, got %q", out)
	}
}

// A failed direct write surfaces as an error instead of being discarded.
func TestRecordReportsDirectWriteFailure(t *testing.T) {
	var buf bytes.Buffer
	configureLogs(t, logs.Disabled, &buf)
	auditMu.Lock()
	auditFallback = failingWriter{}
	auditMu.Unlock()
	t.Cleanup(func() {
		auditMu.Lock()
		auditFallback = nil
		auditMu.Unlock()
	})
	if err := Recordf(AuditWarn, "admin.audit probe=%d", 2); err == nil {
		t.Fatal("expected write failure to be returned")
	}
}
