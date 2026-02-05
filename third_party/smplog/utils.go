package logs

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// types
type ServiceLog struct {
	ts  time.Time
	msg string
}

type ServiceLogger struct {
	logs map[string]*ServiceLog
}

var logger *ServiceLogger = &ServiceLogger{
	logs: make(map[string]*ServiceLog),
}

const (
	// Reset
	StyleReset = "\033[0m"

	// Text attributes
	StyleBold      = "\033[1m"
	StyleDim       = "\033[2m"
	StyleItalic    = "\033[3m" // Not always supported
	StyleUnderline = "\033[4m"
	StyleBlink     = "\033[5m"
	StyleReverse   = "\033[7m"
	StyleHidden    = "\033[8m"
	StyleStrike    = "\033[9m" // Strikethrough

	// Foreground (normal colors)
	StyleBlack   = "\033[30m"
	StyleRed     = "\033[31m"
	StyleGreen   = "\033[32m"
	StyleYellow  = "\033[33m"
	StyleBlue    = "\033[34m"
	StyleMagenta = "\033[35m"
	StyleCyan    = "\033[36m"
	StyleWhite   = "\033[37m"

	// Foreground (bright colors)
	StyleBrightBlack   = "\033[90m" // often used as gray
	StyleBrightRed     = "\033[91m"
	StyleBrightGreen   = "\033[92m"
	StyleBrightYellow  = "\033[93m"
	StyleBrightBlue    = "\033[94m"
	StyleBrightMagenta = "\033[95m"
	StyleBrightCyan    = "\033[96m"
	StyleBrightWhite   = "\033[97m"

	// Background (normal colors)
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"

	// Background (bright colors)
	BgBrightBlack   = "\033[100m"
	BgBrightRed     = "\033[101m"
	BgBrightGreen   = "\033[102m"
	BgBrightYellow  = "\033[103m"
	BgBrightBlue    = "\033[104m"
	BgBrightMagenta = "\033[105m"
	BgBrightCyan    = "\033[106m"
	BgBrightWhite   = "\033[107m"
)

// checks if the log message contains any of the specified filters
// returns true if it does, false otherwise
func LogFilter(format string, filters ...string) bool {
	for _, filter := range filters {
		if strings.Contains(format, filter) {
			return true
		}
	}
	return false
}

// trims the path to a maximum length, prefixing with "..." if it exceeds the limit
func FormatPath(path string, maxLength int) string {
	const ellipsis = "..."
	if len(path) <= maxLength {
		return fmt.Sprintf("%*s", maxLength, path) // pad left if short
	}

	// Trim from the left, prepend ellipsis
	trimStart := len(path) - (maxLength - len(ellipsis))
	if trimStart < 0 {
		trimStart = 0
	}
	return ellipsis + path[trimStart:]
}

// trims the file path to start from "dps_http/"
func TrimToProjectRoot(root, path string) string {
	root = root + "/"
	idx := strings.Index(path, root)
	if idx == -1 {
		return path // fallback to full path if not found
	}
	return FormatPath(path[idx:], 32)
}

// strips ANSI escape codes from a string
// useful for cleaning colors from logs
func StripANSI(s string) string {
	return regexp.MustCompile(`\x1b\[[0-9;]*m`).ReplaceAllString(s, "")
}

// centers the tag within a given width, padding with spaces
func CenterTag(tag string, width int) string {
	visible := StripANSI(tag)
	tagLen := len(visible)
	if tagLen >= width {
		return tag
	}

	padding := width - tagLen
	left := padding / 2
	right := padding - left

	return strings.Repeat("", left) + tag + strings.Repeat(" ", right)
}

func ColorText(color, text string) string {
	return fmt.Sprintf("%s%s%s", color, text, StyleReset)
}

// 256-color helpers (use values 0-255)
func StyleColor256(n int) string {
	return fmt.Sprintf("\033[38;5;%dm", n)
}

func BgColor256(n int) string {
	return fmt.Sprintf("\033[48;5;%dm", n)
}
