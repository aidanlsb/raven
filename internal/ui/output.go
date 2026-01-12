package ui

import "fmt"

// Unicode symbols for status indicators
const (
	SymbolSuccess = "✓"
	SymbolError   = "✗"
	SymbolWarning = "⚠"
	SymbolInfo    = "ℹ"
)

// Success returns a success message with checkmark symbol
func Success(msg string) string {
	return fmt.Sprintf("%s %s", SymbolSuccess, msg)
}

// Successf returns a formatted success message with checkmark symbol
func Successf(format string, args ...interface{}) string {
	return Success(fmt.Sprintf(format, args...))
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
