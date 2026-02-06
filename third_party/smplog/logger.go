package logs

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rs/zerolog"
)

const (
	// INACTIVE disables all legacy helper output.
	INACTIVE = iota
	// ERROR enables only error-level helpers.
	ERROR
	// INFO enables info-level helpers.
	INFO
	// WARN enables warning-level helpers.
	WARN
	// DEBUG enables debug-level helpers.
	DEBUG
	// DIAGNOSTICS enables trace and debug helper output.
	DIAGNOSTICS
)

var (
	// MODE keeps compatibility with the previous threshold API.
	MODE = WARN
	// TRACE enables file:line prefix in String/Print helpers.
	TRACE = false
	// LOGGER_enable_timestamp enables timestamp prefix in String/Print helpers.
	LOGGER_enable_timestamp = false
)

// Config controls smplog and zerolog behavior.
type Config struct {
	// Writer is the final output destination.
	Writer io.Writer
	// Level is the logger and global threshold.
	Level Level
	// Timestamp appends timestamp field to log context.
	Timestamp bool
	// Caller appends caller field to log context.
	Caller bool
	// Stack appends stack traces when Stack() is used on events.
	Stack bool
	// TimeFormat controls zerolog timestamp formatting.
	TimeFormat string
	// NoColor disables console colors in wrapper mode.
	NoColor bool
	// Bypass disables all console wrapping and emits plain zerolog output.
	Bypass bool
	// Colors controls text colors in console mode.
	Colors ConsoleColors
	// ConfigureZerolog allows global zerolog customization before logger setup.
	ConfigureZerolog func()
	// ConfigureConsole allows direct edits to the console writer.
	ConfigureConsole func(w *ConsoleWriter)
	// ConfigureLogger allows final logger customization before install.
	ConfigureLogger func(l Logger) Logger
}

var (
	stateMu       sync.RWMutex
	currentConfig Config
	currentLogger *Logger
)

func init() {
	Configure(DefaultConfig())
}

// DefaultConfig returns a sensible console-focused default config.
func DefaultConfig() Config {
	return Config{
		Writer:     os.Stdout,
		Level:      modeToLevel(MODE),
		Timestamp:  true,
		TimeFormat: time.RFC3339,
		NoColor:    false,
		Bypass:     false,
		Colors:     DefaultColors(),
	}
}

// Configure applies config and replaces the package-global logger.
func Configure(cfg Config) {
	stateMu.Lock()
	defer stateMu.Unlock()
	currentConfig = normalizeConfig(cfg)
	logger := buildLogger(currentConfig)
	currentLogger = &logger
}

// Configured returns the currently installed logger config.
func Configured() Config {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return currentConfig
}

// SetBypass toggles wrapper bypass mode.
func SetBypass(enabled bool) {
	cfg := Configured()
	cfg.Bypass = enabled
	Configure(cfg)
}

// SetColors updates console colors.
func SetColors(colors ConsoleColors) {
	cfg := Configured()
	cfg.Colors = colors
	Configure(cfg)
}

// SetMode updates MODE and maps it into zerolog level.
func SetMode(mode int) {
	MODE = mode
	SetLevel(modeToLevel(mode))
}

// SetLevel updates logger and global zerolog level.
func SetLevel(level Level) {
	cfg := Configured()
	cfg.Level = level
	Configure(cfg)
}

// SetLogger replaces the package-global logger directly.
func SetLogger(l Logger) {
	stateMu.Lock()
	defer stateMu.Unlock()
	currentLogger = &l
}

// Zerolog returns the active package-global logger.
func Zerolog() *Logger {
	stateMu.RLock()
	defer stateMu.RUnlock()
	return currentLogger
}

// With returns a context builder from the active logger.
func With() Context {
	return Zerolog().With()
}

// AtLevel returns a level-scoped event from the active logger.
func AtLevel(level Level) *Event {
	return Zerolog().WithLevel(zerolog.Level(level))
}

func normalizeConfig(cfg Config) Config {
	if cfg.Writer == nil {
		cfg.Writer = os.Stdout
	}
	if cfg.TimeFormat == "" {
		cfg.TimeFormat = time.RFC3339
	}
	if cfg.Colors == (ConsoleColors{}) {
		cfg.Colors = DefaultColors()
	}
	return cfg
}

func buildLogger(cfg Config) Logger {
	if cfg.ConfigureZerolog != nil {
		cfg.ConfigureZerolog()
	}

	zerolog.TimeFieldFormat = cfg.TimeFormat
	zerolog.SetGlobalLevel(cfg.Level)

	writer := cfg.Writer
	if !cfg.Bypass {
		console := ConsoleWriter{
			Out:        cfg.Writer,
			NoColor:    cfg.NoColor,
			TimeFormat: cfg.TimeFormat,
		}
		applyConsoleFormatting(&console, cfg)
		if cfg.ConfigureConsole != nil {
			cfg.ConfigureConsole(&console)
		}
		writer = console
	}

	logger := zerolog.New(writer).Level(cfg.Level)
	ctx := logger.With()
	if cfg.Timestamp {
		ctx = ctx.Timestamp()
	}
	if cfg.Caller {
		ctx = ctx.Caller()
	}
	if cfg.Stack {
		ctx = ctx.Stack()
	}
	logger = ctx.Logger()

	if cfg.ConfigureLogger != nil {
		logger = cfg.ConfigureLogger(logger)
	}

	return logger
}

func applyConsoleFormatting(console *ConsoleWriter, cfg Config) {
	console.FormatPrepare = func(evt map[string]interface{}) error {
		level := strings.ToLower(fmt.Sprint(evt[zerolog.LevelFieldName]))
		if raw, ok := evt[zerolog.LevelFieldName]; ok {
			evt[zerolog.LevelFieldName] = colorize(
				cfg.Colors.level(level),
				strings.ToUpper(fmt.Sprint(raw)),
				cfg.NoColor,
			)
		}
		if raw, ok := evt[zerolog.MessageFieldName]; ok {
			messageColor := cfg.Colors.Message
			if messageColor == "" {
				messageColor = cfg.Colors.level(level)
			}
			evt[zerolog.MessageFieldName] = colorize(
				messageColor,
				fmt.Sprint(raw),
				cfg.NoColor,
			)
		}
		return nil
	}

	console.FormatTimestamp = func(i interface{}) string {
		return colorize(cfg.Colors.Timestamp, fmt.Sprint(i), cfg.NoColor)
	}
	console.FormatLevel = func(i interface{}) string {
		return fmt.Sprint(i)
	}
	console.FormatFieldName = func(i interface{}) string {
		return colorize(cfg.Colors.FieldName, fmt.Sprint(i), cfg.NoColor)
	}
	console.FormatFieldValue = func(i interface{}) string {
		return colorize(cfg.Colors.FieldValue, fmt.Sprint(i), cfg.NoColor)
	}
	console.FormatErrFieldName = console.FormatFieldName
	console.FormatErrFieldValue = console.FormatFieldValue
}

func modeToLevel(mode int) Level {
	switch mode {
	case INACTIVE:
		return Disabled
	case ERROR:
		return ErrorLevel
	case INFO:
		return InfoLevel
	case WARN:
		return WarnLevel
	case DEBUG:
		return DebugLevel
	case DIAGNOSTICS:
		return TraceLevel
	default:
		return WarnLevel
	}
}

// ColorTest prints one message per level using current configuration.
func ColorTest() {
	AtLevel(TraceLevel).Msg("trace")
	AtLevel(DebugLevel).Msg("debug")
	AtLevel(InfoLevel).Msg("info")
	AtLevel(WarnLevel).Msg("warn")
	AtLevel(ErrorLevel).Msg("error")
}

// Log logs a message at info level.
func Log(msg string) {
	Zerolog().Info().Msg(msg)
}

// Logf logs a formatted message at info level.
func Logf(format string, v ...any) {
	Zerolog().Info().Msgf(format, v...)
}

// Fatal logs a message at fatal level and exits.
func Fatal(msg string) {
	Zerolog().Fatal().Msg(msg)
}

// Fatalf logs a formatted fatal message and exits.
func Fatalf(format string, v ...any) {
	Zerolog().Fatal().Msgf(format, v...)
}

// Err logs a message at error level.
func Err(msg string) {
	Zerolog().Error().Msg(msg)
}

// Errf logs a formatted message at error level.
func Errf(format string, v ...any) {
	Zerolog().Error().Msgf(format, v...)
}

// Warn logs a message at warn level.
func Warn(msg string) {
	Zerolog().Warn().Msg(msg)
}

// Warnf logs a formatted message at warn level.
func Warnf(format string, v ...any) {
	Zerolog().Warn().Msgf(format, v...)
}

// Info logs a message at info level.
func Info(msg string) {
	Zerolog().Info().Msg(msg)
}

// Infof logs a formatted message at info level.
func Infof(format string, v ...any) {
	Zerolog().Info().Msgf(format, v...)
}

// Debug logs a message at debug level.
func Debug(msg string) {
	Zerolog().Debug().Msg(msg)
}

// Debugf logs a formatted message at debug level.
func Debugf(format string, v ...any) {
	Zerolog().Debug().Msgf(format, v...)
}

// Dev logs a message at debug level for legacy compatibility.
func Dev(msg string) {
	Zerolog().Debug().Msg(msg)
}

// Devf logs a formatted message at debug level for legacy compatibility.
func Devf(format string, v ...any) {
	Zerolog().Debug().Msgf(format, v...)
}

// Init logs a message at trace level for legacy compatibility.
func Init(msg string) {
	Zerolog().Trace().Msg(msg)
}

// Initf logs a formatted message at trace level.
func Initf(format string, v ...any) {
	Zerolog().Trace().Msgf(format, v...)
}

// MsgSuccess logs an informational success message.
func MsgSuccess(msg string) {
	Print(StyleColor256(66), "[MDS]", "%s", msg)
}

// MsgSuccessf logs a formatted informational success message.
func MsgSuccessf(format string, v ...any) {
	MsgSuccess(fmt.Sprintf(format, v...))
}

// MsgFailure logs an informational failure message.
func MsgFailure(msg string) {
	Print(StyleColor256(130), "[MDF]", "%s", msg)
}

// MsgFailuref logs a formatted informational failure message.
func MsgFailuref(format string, v ...any) {
	MsgFailure(fmt.Sprintf(format, v...))
}

// Print logs a formatted message with optional tag and ANSI color.
func Print(color, tag, format string, v ...any) {
	msg := String(color, tag, format, v...)
	if !Configured().Bypass {
		msg = ColorText(color, msg)
	}
	Zerolog().Info().Msg(msg)
}

// String formats a message with legacy TRACE and timestamp prefixes.
func String(_color, tag, format string, v ...any) string {
	msg := fmt.Sprintf(format, v...)
	if tag != "" {
		msg = fmt.Sprintf("%s %s", tag, msg)
	}
	if !TRACE {
		if LOGGER_enable_timestamp {
			return fmt.Sprintf("%s %s", time.Now().Format(time.Stamp), msg)
		}
		return msg
	}

	_, file, line, ok := runtime.Caller(2)
	if !ok {
		return msg
	}

	path := FormatPath(file, 32)
	if LOGGER_enable_timestamp {
		return fmt.Sprintf("%s [%s:%d] %s", time.Now().Format(time.Stamp), path, line, msg)
	}
	return fmt.Sprintf("[%s:%d] %s", path, line, msg)
}
