package ui

import (
	"fmt"
	"strings"
)

// Symbols for status indicators
const (
	SymbolCheck     = "✓" // Explicit action success (created, added, etc.)
	SymbolStar      = "✦" // Concluding/summary messages (no issues, results)
	SymbolError     = "✗"
	SymbolWarning   = "!"
	SymbolInfo      = "*"
	SymbolUnchecked = "□"
	SymbolChecked   = "☑"
	SymbolDot       = "·"
	SymbolDash      = "—"
	SymbolAttention = "»"
)

// Check returns a message with checkmark symbol (for explicit action success)
func Check(msg string) string {
	return fmt.Sprintf("%s %s", SymbolCheck, msg)
}

// Checkf returns a formatted message with checkmark symbol
func Checkf(format string, args ...interface{}) string {
	return Check(fmt.Sprintf(format, args...))
}

// Star returns a message with star symbol (for concluding/summary messages)
func Star(msg string) string {
	return fmt.Sprintf("%s %s", SymbolStar, msg)
}

// Starf returns a formatted message with star symbol
func Starf(format string, args ...interface{}) string {
	return Star(fmt.Sprintf(format, args...))
}

// Error returns an error message with X symbol
func Error(msg string) string {
	return fmt.Sprintf("%s %s", SymbolError, msg)
}

// Errorf returns a formatted error message with X symbol
func Errorf(format string, args ...interface{}) string {
	return Error(fmt.Sprintf(format, args...))
}

// Warning returns a warning message with warning symbol
func Warning(msg string) string {
	return fmt.Sprintf("%s %s", SymbolWarning, msg)
}

// Warningf returns a formatted warning message with warning symbol
func Warningf(format string, args ...interface{}) string {
	return Warning(fmt.Sprintf(format, args...))
}

// Info returns an info message with info symbol
func Info(msg string) string {
	return fmt.Sprintf("%s %s", SymbolInfo, msg)
}

// Infof returns a formatted info message with info symbol
func Infof(format string, args ...interface{}) string {
	return Info(fmt.Sprintf(format, args...))
}

// Header returns a styled section header
func Header(msg string) string {
	return Bold.Render(msg)
}

// FilePath returns an accent-styled file path
func FilePath(path string) string {
	return Accent.Render(path)
}

// LineNum returns a muted line number
func LineNum(n int) string {
	return Muted.Render(fmt.Sprintf("%d", n))
}

// LineNumPadded returns a muted, right-padded line number
func LineNumPadded(n int, width int) string {
	return Muted.Render(fmt.Sprintf("%*d", width, n))
}

// Hint returns muted hint text
func Hint(msg string) string {
	return Muted.Render(msg)
}

// Count returns a styled count badge (e.g., "(3 errors)")
func Count(n int, singular, plural string) string {
	if n == 1 {
		return fmt.Sprintf("(%d %s)", n, singular)
	}
	return fmt.Sprintf("(%d %s)", n, plural)
}

// ErrorWarningCounts returns a formatted count string like "(3 errors, 2 warnings)"
func ErrorWarningCounts(errors, warnings int) string {
	if errors > 0 && warnings > 0 {
		return fmt.Sprintf("(%d %s, %d %s)",
			errors, pluralize("error", errors),
			warnings, pluralize("warning", warnings))
	} else if errors > 0 {
		return Count(errors, "error", "errors")
	}
	return Count(warnings, "warning", "warnings")
}

// pluralize returns singular or plural form based on count
func pluralize(singular string, count int) string {
	if count == 1 {
		return singular
	}
	return singular + "s"
}

// Divider returns a section divider like "— tasks ————————————————"
func Divider(label string, width int) string {
	if label == "" {
		return Muted.Render(strings.Repeat(SymbolDash, width))
	}
	// Format: "— label ————————"
	prefix := SymbolDash + " " + label + " "
	remaining := width - len(prefix)
	if remaining < 3 {
		remaining = 3
	}
	return Muted.Render(prefix + strings.Repeat(SymbolDash, remaining))
}

// Badge returns a styled badge/pill like "[today]" or "(3)"
func Badge(text string) string {
	return Muted.Render("[" + text + "]")
}

// Metadata returns dot-separated metadata like "status: active · due: 2025-02-15"
func Metadata(pairs ...string) string {
	return Muted.Render(strings.Join(pairs, " " + SymbolDot + " "))
}

// MetadataItem returns a single "key: value" metadata item
func MetadataItem(key, value string) string {
	return key + ": " + value
}

// Checkbox returns a checkbox symbol based on checked state
func Checkbox(checked bool) string {
	if checked {
		return SymbolChecked
	}
	return SymbolUnchecked
}

// Indent returns a string indented by n spaces
func Indent(n int, text string) string {
	return strings.Repeat(" ", n) + text
}

// ResultCount returns a muted result count like "2 results"
func ResultCount(n int) string {
	return Muted.Render(fmt.Sprintf("%d %s", n, pluralize("result", n)))
}
