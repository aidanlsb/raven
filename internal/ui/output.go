package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"

	"github.com/aidanlsb/raven/internal/parser"
)

// VisibleLen returns the visible length of a string, excluding ANSI escape codes
func VisibleLen(s string) int {
	return ansi.StringWidth(s)
}

// Symbols for status indicators
const (
	SymbolCheck     = "✓" // Explicit action success (created, added, etc.)
	SymbolStar      = "✦" // Concluding/summary messages (no issues, results)
	SymbolError     = "✗"
	SymbolWarning   = "!"
	SymbolInfo      = "*"
	SymbolDot       = "•"
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
	return Accent.Render(msg)
}

// SectionHeader returns a styled section header with a leading symbol.
func SectionHeader(msg string) string {
	return Header(SymbolStar + " " + msg)
}

// FilePath returns a styled file path
func FilePath(path string) string {
	return Bold.Render(path)
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
	return dividerWithRender(label, width, Muted.Render)
}

// DividerWithAccentLabel renders a divider with muted dashes and an accent label.
func DividerWithAccentLabel(label string, width int) string {
	if label == "" {
		return Muted.Render(strings.Repeat(SymbolDash, width))
	}

	// Format: "— " + accent(label) + " ————————"
	left := SymbolDash + " "
	rightPrefix := " "
	remaining := width - len(left) - len(label) - len(rightPrefix)
	if remaining < 3 {
		remaining = 3
	}

	return Muted.Render(left) + Accent.Render(label) + Muted.Render(rightPrefix+strings.Repeat(SymbolDash, remaining))
}

func dividerWithRender(label string, width int, render func(...string) string) string {
	if render == nil {
		render = func(parts ...string) string {
			if len(parts) == 0 {
				return ""
			}
			return parts[0]
		}
	}
	if label == "" {
		return render(strings.Repeat(SymbolDash, width))
	}
	// Format: "— label ————————"
	prefix := SymbolDash + " " + label + " "
	remaining := width - len(prefix)
	if remaining < 3 {
		remaining = 3
	}
	return render(prefix + strings.Repeat(SymbolDash, remaining))
}

// Badge returns a styled badge/pill like "[today]" or "(3)"
func Badge(text string) string {
	return Muted.Render("[" + text + "]")
}

// Bullet returns a bullet-prefixed item string.
func Bullet(msg string) string {
	return fmt.Sprintf("%s %s", Muted.Render(SymbolDot), msg)
}

// Indent returns a string indented by n spaces
func Indent(n int, text string) string {
	return strings.Repeat(" ", n) + text
}

// Trait formats a trait with styling.
// The @ and trait name use syntax styling, and the value (if any) uses
// a subtle syntax style.
func Trait(traitType string, value string) string {
	if value == "" {
		return Syntax.Render("@" + traitType)
	}
	return Syntax.Render("@"+traitType) + SyntaxSubtle.Render("("+value+")")
}

// HighlightTraits highlights all @trait patterns in text.
// Traits are bold, values are muted.
// Uses parser.TraitHighlightPattern as the canonical pattern for trait matching.
func HighlightTraits(text string) string {
	return parser.TraitHighlightPattern.ReplaceAllStringFunc(text, func(match string) string {
		parts := parser.TraitHighlightPattern.FindStringSubmatch(match)
		if len(parts) < 2 {
			return match
		}
		traitName := parts[1]
		value := ""
		if len(parts) >= 3 {
			value = parts[2]
		}
		return Trait(traitName, value)
	})
}

// FieldChange formats a field change as "field: old → new"
func FieldChange(field, oldValue, newValue string) string {
	return fmt.Sprintf("%s: %s → %s",
		field,
		Muted.Render(oldValue),
		Bold.Render(newValue))
}

// FieldSet formats a new field value as "field: value"
func FieldSet(field, value string) string {
	return fmt.Sprintf("%s: %s", field, Bold.Render(value))
}

// FieldAdd formats an added field as "+ field: value"
func FieldAdd(field, value string) string {
	return fmt.Sprintf("+ %s: %s", field, Bold.Render(value))
}
