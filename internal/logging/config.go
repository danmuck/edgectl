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

type Profile int

const (
	ProfileRuntime Profile = iota
	ProfileTest
)

var configureOnce sync.Once

func ConfigureRuntime() {
	Configure(ProfileRuntime)
}

func ConfigureTests() {
	Configure(ProfileTest)
}

func Configure(profile Profile) {
	configureOnce.Do(func() {
		cfg := defaultConfig(profile)
		applyEnvOverrides(&cfg)
		logs.Configure(cfg)
	})
}

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
