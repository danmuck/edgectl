package logs

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"time"
)

const (
	INACTIVE    = iota // no logging enabled
	ERROR              // errors only
	INFO               // development info
	WARN               // include warnings
	DEBUG              // include debug info
	DIAGNOSTICS        // all logging enabled
)

// TODO: Move config
const (
	MODE                    = WARN  // Set the logging mode, e.g., DEV, ERROR, WARN, DEBUG
	TRACE                   = false // Enable/disable stack trace logging
	LOGGER_enable_timestamp = false // enables service for default Log()
	// LOGGER_filter           = []string{"api:users"}
)

var (
	LOGGER_service_map = map[string]string{
		"api":     "api",
		"users":   "users",
		"metrics": "metrics",
		"auth":    "auth",
	}
)

func ColorTest() {
	println("\n== Color Test ==")
	Init("This is an init message")
	Err("This is an error message")
	Warn("This is a warning message")
	Info("This is an info message")
	Debug("This is a debug message")
	Fatal("This is a fatal message")
	Devf("Dev mode: %d | expects: %d", MODE, WARN)
	MsgSuccess("Message Successful ie. 200")
	MsgFailure("Message Failure ie. 404")
	println()
}
func init() {
	ColorTest()
}

// Log is message-only (safe)
func Log(msg string) {
	log := &ServiceLog{
		ts:  time.Now(),
		msg: msg,
	}
	for key, value := range LOGGER_service_map {
		if strings.Contains(msg, value) {
			logger.logs[key] = log
		}
	}
	Print(StyleWhite, "[logs]", "%s", msg)
}

// Logf is printf-style
func Logf(format string, v ...any) {
	Log(fmt.Sprintf(format, v...))
}

func Fatal(msg string) {
	Print(StyleColor256(196), "[fatal]", "%s", msg)
	if msg == "This is a fatal message" {
		return
	}
	os.Exit(1)
}
func Fatalf(format string, v ...any) { Fatal(fmt.Sprintf(format, v...)) }

func Err(msg string) {
	if MODE < ERROR {
		return
	}
	Print(StyleRed, "[error]", "%s", msg)
}
func Errf(format string, v ...any) { Err(fmt.Sprintf(format, v...)) }

func Warn(msg string) {
	if MODE < WARN {
		return
	}
	Print(StyleYellow, "[warn]", "%s", msg)
}
func Warnf(format string, v ...any) { Warn(fmt.Sprintf(format, v...)) }

func Info(msg string) {
	if MODE < INFO {
		return
	}
	Print(StyleBlue, "[info]", "%s", msg)
}
func Infof(format string, v ...any) { Info(fmt.Sprintf(format, v...)) }

func Debug(msg string) {
	if MODE < DEBUG {
		return
	}
	Print(StyleGreen, "[debug]", "%s", msg)
}
func Debugf(format string, v ...any) { Debug(fmt.Sprintf(format, v...)) }

func Dev(msg string) {
	Print(StyleColor256(89), "[dev_]", "%s", msg)
}
func Devf(format string, v ...any) { Dev(fmt.Sprintf(format, v...)) }

func Init(msg string) {
	if MODE < DIAGNOSTICS {
		return
	}
	Print(StyleBlack, "[init]", "%s", msg)
}
func Initf(format string, v ...any) { Init(fmt.Sprintf(format, v...)) }

func MsgSuccess(msg string)               { Print(StyleColor256(66), "[MDS]", "%s", msg) }
func MsgSuccessf(format string, v ...any) { MsgSuccess(fmt.Sprintf(format, v...)) }

func MsgFailure(msg string)               { Print(StyleColor256(130), "[MDF]", "%s", msg) }
func MsgFailuref(format string, v ...any) { MsgFailure(fmt.Sprintf(format, v...)) }

// T is the type of log, e.g. "dev", "error", "warn", etc.
// format, v... are the format string and values to Print
func Print(C, T, format string, v ...any) {
	if LOGGER_enable_timestamp {
		fmt.Println(ColorText(C, String(C, T, format, v...)))
	} else {
		fmt.Println(ColorText(C, String(C, T, format, v...)))
	}

}

func String(C, T, format string, v ...any) string {
	msg := fmt.Sprintf(format, v...)
	ts := time.Now().Format(time.Stamp)
	if !TRACE {
		// If TRACE is disabled we don't include the file and line number
		return fmt.Sprintf(format, v...)
	}
	_, file, line, ok := runtime.Caller(3)
	if ok {
		path := TrimToProjectRoot("dps_infra", file)           // max 32 chars
		tag := CenterTag(T, 9)                                 // padded 9 and centered tag
		lineStr := fmt.Sprintf(":%4d", line)                   // pad line number
		prefix := fmt.Sprintf("%s[%s%s] ", tag, path, lineStr) // final prefix
		if LOGGER_enable_timestamp {
			return fmt.Sprintf("%s %s%s", ts, prefix, msg)
		}
		return fmt.Sprintf("%s%s", prefix, msg)
	}
	return msg
}
