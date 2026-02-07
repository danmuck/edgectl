package logging

import (
	"os"
	"strconv"
	"strings"
	"sync"

	logs "github.com/danmuck/smplog"
)

const (
	EnvLogLevel     = "EDGECTL_LOG_LEVEL"
	EnvLogTimestamp = "EDGECTL_LOG_TIMESTAMP"
	EnvLogNoColor   = "EDGECTL_LOG_NOCOLOR"
	EnvLogBypass    = "EDGECTL_LOG_BYPASS"
)

// Logging profile selector for runtime vs test defaults.
type Profile int

const (
	ProfileRuntime Profile = iota
	ProfileTest
)

var configureOnce sync.Once

// Logging runtime initializer for one-time runtime defaults.
func ConfigureRuntime() {
	Configure(ProfileRuntime)
}

// Logging test initializer for one-time test defaults.
func ConfigureTests() {
	Configure(ProfileTest)
}

// Logging one-time process initializer.
func Configure(profile Profile) {
	configureOnce.Do(func() {
		cfg := defaultConfig(profile)
		applyEnvOverrides(&cfg)
		logs.Configure(cfg)
	})
}

// Logging default-config builder for selected profile.
func defaultConfig(profile Profile) logs.Config {
	cfg := logs.DefaultConfig()
	switch profile {
	case ProfileTest:
		cfg.Level = logs.DebugLevel
		cfg.Timestamp = false
	default:
		cfg.Level = logs.InfoLevel
		cfg.Timestamp = true
	}
	return cfg
}

// Logging env overlay for EDGECTL_LOG_* settings.
func applyEnvOverrides(cfg *logs.Config) {
	if lvl, ok := parseLevel(os.Getenv(EnvLogLevel)); ok {
		cfg.Level = lvl
	}
	if v, ok := parseBool(os.Getenv(EnvLogTimestamp)); ok {
		cfg.Timestamp = v
	}
	if v, ok := parseBool(os.Getenv(EnvLogNoColor)); ok {
		cfg.NoColor = v
	}
	if v, ok := parseBool(os.Getenv(EnvLogBypass)); ok {
		cfg.Bypass = v
	}
}

// Logging level parser for user-provided level values.
func parseLevel(raw string) (logs.Level, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "":
		return logs.InfoLevel, false
	case "trace", "diagnostics":
		return logs.TraceLevel, true
	case "debug":
		return logs.DebugLevel, true
	case "info":
		return logs.InfoLevel, true
	case "warn", "warning":
		return logs.WarnLevel, true
	case "error":
		return logs.ErrorLevel, true
	case "disabled", "disable", "off", "none", "inactive":
		return logs.Disabled, true
	default:
		return logs.InfoLevel, false
	}
}

// Logging bool parser for optional environment values.
func parseBool(raw string) (bool, bool) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return false, false
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, false
	}
	return v, true
}
