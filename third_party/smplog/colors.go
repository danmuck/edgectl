package logs

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	// StyleReset clears all ANSI attributes.
	StyleReset = "\033[0m"

	// Text attributes.
	StyleBold      = "\033[1m"
	StyleDim       = "\033[2m"
	StyleItalic    = "\033[3m"
	StyleUnderline = "\033[4m"
	StyleBlink     = "\033[5m"
	StyleReverse   = "\033[7m"
	StyleHidden    = "\033[8m"
	StyleStrike    = "\033[9m"

	// Foreground colors.
	StyleBlack   = "\033[30m"
	StyleRed     = "\033[31m"
	StyleGreen   = "\033[32m"
	StyleYellow  = "\033[33m"
	StyleBlue    = "\033[34m"
	StyleMagenta = "\033[35m"
	StyleCyan    = "\033[36m"
	StyleWhite   = "\033[37m"

	// Bright foreground colors.
	StyleBrightBlack   = "\033[90m"
	StyleBrightRed     = "\033[91m"
	StyleBrightGreen   = "\033[92m"
	StyleBrightYellow  = "\033[93m"
	StyleBrightBlue    = "\033[94m"
	StyleBrightMagenta = "\033[95m"
	StyleBrightCyan    = "\033[96m"
	StyleBrightWhite   = "\033[97m"

	// Background colors.
	BgBlack   = "\033[40m"
	BgRed     = "\033[41m"
	BgGreen   = "\033[42m"
	BgYellow  = "\033[43m"
	BgBlue    = "\033[44m"
	BgMagenta = "\033[45m"
	BgCyan    = "\033[46m"
	BgWhite   = "\033[47m"

	// Bright background colors.
	BgBrightBlack   = "\033[100m"
	BgBrightRed     = "\033[101m"
	BgBrightGreen   = "\033[102m"
	BgBrightYellow  = "\033[103m"
	BgBrightBlue    = "\033[104m"
	BgBrightMagenta = "\033[105m"
	BgBrightCyan    = "\033[106m"
	BgBrightWhite   = "\033[107m"
)

var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// ConsoleColors defines ANSI colors applied by the console wrapper.
type ConsoleColors struct {
	Trace      string
	Debug      string
	Info       string
	Warn       string
	Error      string
	Fatal      string
	Panic      string
	Message    string
	Timestamp  string
	FieldName  string
	FieldValue string
}

// DefaultColors returns the default level-based color palette.
func DefaultColors() ConsoleColors {
	return ConsoleColors{
		Trace:      StyleBrightBlack,
		Debug:      StyleGreen,
		Info:       StyleBlue,
		Warn:       StyleYellow,
		Error:      StyleRed,
		Fatal:      StyleColor256(196),
		Panic:      StyleMagenta,
		Timestamp:  StyleBrightBlack,
		FieldName:  StyleCyan,
		FieldValue: StyleWhite,
	}
}

// NoColors returns a palette with no color overrides.
func NoColors() ConsoleColors {
	return ConsoleColors{}
}

func (c ConsoleColors) level(level string) string {
	switch strings.ToLower(level) {
	case "trace":
		return c.Trace
	case "debug":
		return c.Debug
	case "info":
		return c.Info
	case "warn", "warning":
		return c.Warn
	case "error":
		return c.Error
	case "fatal":
		return c.Fatal
	case "panic":
		return c.Panic
	default:
		return ""
	}
}

func colorize(color, text string, disabled bool) string {
	if disabled || color == "" || text == "" {
		return text
	}
	return fmt.Sprintf("%s%s%s", color, text, StyleReset)
}

// ColorText wraps text with ANSI color and reset.
func ColorText(color, text string) string {
	return colorize(color, text, false)
}

// StyleColor256 returns a 256-color ANSI foreground style.
func StyleColor256(n int) string {
	return fmt.Sprintf("\033[38;5;%dm", n)
}

// BgColor256 returns a 256-color ANSI background style.
func BgColor256(n int) string {
	return fmt.Sprintf("\033[48;5;%dm", n)
}

// StripANSI removes ANSI control codes from a string.
func StripANSI(s string) string {
	return ansiPattern.ReplaceAllString(s, "")
}

// CenterTag centers a tag to width while keeping ANSI codes intact.
func CenterTag(tag string, width int) string {
	visible := StripANSI(tag)
	tagLen := len(visible)
	if tagLen >= width {
		return tag
	}

	padding := width - tagLen
	left := padding / 2
	right := padding - left

	return strings.Repeat(" ", left) + tag + strings.Repeat(" ", right)
}

// FormatPath truncates a path to maxLength from the left.
func FormatPath(path string, maxLength int) string {
	const ellipsis = "..."
	if len(path) <= maxLength {
		return fmt.Sprintf("%*s", maxLength, path)
	}

	trimStart := len(path) - (maxLength - len(ellipsis))
	if trimStart < 0 {
		trimStart = 0
	}
	return ellipsis + path[trimStart:]
}

// TrimToProjectRoot trims file path from the first matching project root.
func TrimToProjectRoot(root, path string) string {
	root = root + "/"
	idx := strings.Index(path, root)
	if idx == -1 {
		return path
	}
	return FormatPath(path[idx:], 32)
}

// LogFilter checks whether format contains one of filters.
func LogFilter(format string, filters ...string) bool {
	for _, filter := range filters {
		if strings.Contains(format, filter) {
			return true
		}
	}
	return false
}
