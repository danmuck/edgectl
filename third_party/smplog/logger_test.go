package logs

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestConfigureAppliesConsoleColors(t *testing.T) {
	var out bytes.Buffer

	Configure(Config{
		Writer:    &out,
		Level:     InfoLevel,
		Timestamp: false,
		Bypass:    false,
		NoColor:   false,
		Colors: ConsoleColors{
			Info:    StyleRed,
			Message: StyleGreen,
		},
	})
	t.Cleanup(func() { Configure(DefaultConfig()) })

	Info("color-check")

	logLine := out.String()
	if !strings.Contains(logLine, StyleRed) {
		t.Fatalf("expected info color %q in output: %q", StyleRed, logLine)
	}
	if !strings.Contains(logLine, StyleGreen) {
		t.Fatalf("expected message color %q in output: %q", StyleGreen, logLine)
	}
}

func TestBypassModeEmitsRawJSON(t *testing.T) {
	var out bytes.Buffer

	Configure(Config{
		Writer:    &out,
		Level:     InfoLevel,
		Timestamp: false,
		Bypass:    true,
	})
	t.Cleanup(func() { Configure(DefaultConfig()) })

	Info("raw-json")

	logLine := strings.TrimSpace(out.String())
	if strings.Contains(logLine, "\x1b[") {
		t.Fatalf("expected no ANSI colors in bypass output: %q", logLine)
	}
	if !json.Valid([]byte(logLine)) {
		t.Fatalf("expected valid JSON in bypass output: %q", logLine)
	}
	if !strings.Contains(logLine, `"message":"raw-json"`) {
		t.Fatalf("expected message field in bypass output: %q", logLine)
	}
}

func TestConfigureLoggerHookIsApplied(t *testing.T) {
	var out bytes.Buffer

	Configure(Config{
		Writer:    &out,
		Level:     InfoLevel,
		Timestamp: false,
		Bypass:    true,
		ConfigureLogger: func(l Logger) Logger {
			return l.With().Str("service", "edge").Logger()
		},
	})
	t.Cleanup(func() { Configure(DefaultConfig()) })

	Info("hook-check")

	logLine := out.String()
	if !strings.Contains(logLine, `"service":"edge"`) {
		t.Fatalf("expected custom logger field in output: %q", logLine)
	}
}
