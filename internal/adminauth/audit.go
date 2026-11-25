package adminauth

import (
	"fmt"
	"io"
	"os"
	"sync"

	logs "github.com/danmuck/smplog"
)

// Audit record severity; selects the log level used when normal logging would emit the record.
type AuditLevel int

const (
	AuditInfo AuditLevel = iota
	AuditWarn
)

var (
	auditMu sync.Mutex
	// Destination for records the configured log level would drop; nil means the logger's writer.
	auditFallback io.Writer
)

// Record writes one audit line (admin_security.toml [audit]). Audit records are a security
// record, not diagnostics: when the configured log level would drop the line (EDGECTL_LOG_LEVEL
// above its level, or logging disabled) it is written directly to the log destination instead.
// A failed direct write is reported on stderr and returned; it is never silently discarded.
func Record(level AuditLevel, line string) error {
	cfg := logs.Configured()
	want := logs.InfoLevel
	if level == AuditWarn {
		want = logs.WarnLevel
	}
	if want >= cfg.Level {
		if level == AuditWarn {
			logs.Warn(line)
		} else {
			logs.Info(line)
		}
		return nil
	}
	auditMu.Lock()
	defer auditMu.Unlock()
	out := auditFallback
	if out == nil {
		out = cfg.Writer
	}
	if out == nil {
		out = os.Stdout
	}
	if _, err := fmt.Fprintf(out, "AUDIT %s\n", line); err != nil {
		fmt.Fprintf(os.Stderr, "adminauth: audit record write failed: %v: %s\n", err, line)
		return fmt.Errorf("adminauth: audit record write failed: %w", err)
	}
	return nil
}

// Recordf formats and writes one audit line; see Record.
func Recordf(level AuditLevel, format string, args ...any) error {
	return Record(level, fmt.Sprintf(format, args...))
}
