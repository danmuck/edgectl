package logs

import (
	"io"
	"time"

	"github.com/rs/zerolog"
)

// Logger aliases zerolog.Logger for users who only import smplog.
type Logger = zerolog.Logger

// Event aliases zerolog.Event for users who only import smplog.
type Event = zerolog.Event

// Context aliases zerolog.Context for users who only import smplog.
type Context = zerolog.Context

// Array aliases zerolog.Array for users who only import smplog.
type Array = zerolog.Array

// ConsoleWriter aliases zerolog.ConsoleWriter.
type ConsoleWriter = zerolog.ConsoleWriter

// LevelWriter aliases zerolog.LevelWriter.
type LevelWriter = zerolog.LevelWriter

// Hook aliases zerolog.Hook.
type Hook = zerolog.Hook

// Sampler aliases zerolog.Sampler.
type Sampler = zerolog.Sampler

// LogObjectMarshaler aliases zerolog.LogObjectMarshaler.
type LogObjectMarshaler = zerolog.LogObjectMarshaler

// LogArrayMarshaler aliases zerolog.LogArrayMarshaler.
type LogArrayMarshaler = zerolog.LogArrayMarshaler

// Level aliases zerolog.Level for users who only import smplog.
type Level = zerolog.Level

const (
	// TraceLevel logs highly verbose tracing data.
	TraceLevel Level = zerolog.TraceLevel
	// DebugLevel logs diagnostic information.
	DebugLevel Level = zerolog.DebugLevel
	// InfoLevel logs normal application events.
	InfoLevel Level = zerolog.InfoLevel
	// WarnLevel logs recoverable issues.
	WarnLevel Level = zerolog.WarnLevel
	// ErrorLevel logs failed operations.
	ErrorLevel Level = zerolog.ErrorLevel
	// FatalLevel logs a fatal error then exits.
	FatalLevel Level = zerolog.FatalLevel
	// PanicLevel logs then panics.
	PanicLevel Level = zerolog.PanicLevel
	// NoLevel logs without an explicit level.
	NoLevel Level = zerolog.NoLevel
	// Disabled disables logging.
	Disabled Level = zerolog.Disabled
)

// New returns a zerolog logger writing to w.
func New(w io.Writer) Logger {
	return zerolog.New(w)
}

// Nop returns a disabled logger.
func Nop() Logger {
	return zerolog.Nop()
}

// NewConsoleWriter creates a zerolog console writer.
func NewConsoleWriter(options ...func(w *ConsoleWriter)) ConsoleWriter {
	return zerolog.NewConsoleWriter(options...)
}

// MultiLevelWriter duplicates writes to multiple targets.
func MultiLevelWriter(writers ...io.Writer) LevelWriter {
	return zerolog.MultiLevelWriter(writers...)
}

// SyncWriter wraps a writer with synchronization.
func SyncWriter(w io.Writer) io.Writer {
	return zerolog.SyncWriter(w)
}

// Dict creates a sub-dictionary event.
func Dict() *Event {
	return zerolog.Dict()
}

// Arr creates a new array helper.
func Arr() *Array {
	return zerolog.Arr()
}

// ParseLevel parses text into a log level.
func ParseLevel(level string) (Level, error) {
	return zerolog.ParseLevel(level)
}

// SetGlobalLevel sets the global threshold.
func SetGlobalLevel(level Level) {
	zerolog.SetGlobalLevel(level)
}

// GlobalLevel returns the current global threshold.
func GlobalLevel() Level {
	return zerolog.GlobalLevel()
}

// SetTimeFieldFormat sets the timestamp format used by zerolog fields.
func SetTimeFieldFormat(format string) {
	zerolog.TimeFieldFormat = format
}

// SetTimestampFieldName sets the zerolog timestamp key.
func SetTimestampFieldName(name string) {
	zerolog.TimestampFieldName = name
}

// SetLevelFieldName sets the zerolog level key.
func SetLevelFieldName(name string) {
	zerolog.LevelFieldName = name
}

// SetMessageFieldName sets the zerolog message key.
func SetMessageFieldName(name string) {
	zerolog.MessageFieldName = name
}

// SetErrorFieldName sets the zerolog error key.
func SetErrorFieldName(name string) {
	zerolog.ErrorFieldName = name
}

// SetCallerFieldName sets the zerolog caller key.
func SetCallerFieldName(name string) {
	zerolog.CallerFieldName = name
}

// SetDurationFieldUnit sets duration unit formatting for zerolog.
func SetDurationFieldUnit(unit time.Duration) {
	zerolog.DurationFieldUnit = unit
}

// SetDurationFieldInteger controls integer duration formatting.
func SetDurationFieldInteger(enabled bool) {
	zerolog.DurationFieldInteger = enabled
}

// SetFloatingPointPrecision sets float formatting precision.
func SetFloatingPointPrecision(precision int) {
	zerolog.FloatingPointPrecision = precision
}

// SetErrorStackMarshaler configures stack marshaling behavior.
func SetErrorStackMarshaler(marshaler func(err error) any) {
	zerolog.ErrorStackMarshaler = marshaler
}

// SetCallerMarshalFunc configures caller rendering.
func SetCallerMarshalFunc(marshal func(pc uintptr, file string, line int) string) {
	zerolog.CallerMarshalFunc = marshal
}
