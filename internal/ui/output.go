package ui

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
)

// ansiPattern matches ANSI escape sequences
var ansiPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

// VisibleLen returns the visible length of a string, excluding ANSI escape codes
func VisibleLen(s string) int {
	return len(ansiPattern.ReplaceAllString(s, ""))
}

// PadRight pads a string to the specified visible width, accounting for ANSI codes
func PadRight(s string, width int) string {
	visible := VisibleLen(s)
	if visible >= width {
		return s
	}
	return s + strings.Repeat(" ", width-visible)
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
	return dividerWithRender(label, width, Muted.Render)
}

// AccentDivider returns a section divider rendered with the accent style.
func AccentDivider(label string, width int) string {
	return dividerWithRender(label, width, Accent.Render)
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

// Metadata returns dot-separated metadata like "status: active · due: 2025-02-15"
func Metadata(pairs ...string) string {
	return Muted.Render(strings.Join(pairs, " "+SymbolDot+" "))
}

// MetadataItem returns a single "key: value" metadata item
func MetadataItem(key, value string) string {
	return key + ": " + value
}

// Bullet returns a bullet-prefixed item string.
func Bullet(msg string) string {
	return fmt.Sprintf("%s %s", Muted.Render(SymbolDot), msg)
}

// Indent returns a string indented by n spaces
func Indent(n int, text string) string {
	return strings.Repeat(" ", n) + text
}

// ResultCount returns a muted result count like "2 results"
func ResultCount(n int) string {
	return Muted.Render(fmt.Sprintf("%d %s", n, pluralize("result", n)))
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

// FieldRemove formats a removed field as "- field: value"
func FieldRemove(field, value string) string {
	return fmt.Sprintf("- %s: %s", field, Muted.Render(value))
}
