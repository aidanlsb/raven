// Package dates provides canonical date/datetime parsing and validation helpers.
//
// This package exists to avoid duplicating date parsing logic across:
// - schema validation
// - vault/CLI date args
// - check validation
// - index/query date filters
// - reference resolution (date shorthand)
package dates

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var (
	dateRegex = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// IsValidDate checks if a string is a valid YYYY-MM-DD date.
func IsValidDate(s string) bool {
	if !dateRegex.MatchString(s) {
		return false
	}
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

// ParseDate parses a YYYY-MM-DD date.
func ParseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if !IsValidDate(s) {
		return time.Time{}, fmt.Errorf("invalid date: %q", s)
	}
	return time.Parse("2006-01-02", s)
}

// IsValidDatetime checks if a string is a valid datetime.
//
// Accepted formats (preserving current behavior):
// - RFC3339 (e.g. 2025-01-01T10:30:00Z, 2025-06-15T14:00:00+05:00)
// - YYYY-MM-DDTHH:MM
// - YYYY-MM-DDTHH:MM:SS
func IsValidDatetime(s string) bool {
	_, err := ParseDatetime(s)
	return err == nil
}

// ParseDatetime parses a datetime in one of the accepted formats.
func ParseDatetime(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, fmt.Errorf("invalid datetime: empty")
	}

	formats := []string{
		time.RFC3339,
		"2006-01-02T15:04",
		"2006-01-02T15:04:05",
	}
	for _, format := range formats {
		if t, err := time.Parse(format, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("invalid datetime: %q", s)
}

// ParseDateArg parses a CLI date argument which can be:
// - "today", "yesterday", "tomorrow" (relative dates)
// - "YYYY-MM-DD" format (absolute date)
// - Empty string defaults to today
func ParseDateArg(arg string, now time.Time) (time.Time, error) {
	if arg == "" {
		return now, nil
	}

	dateArg := strings.ToLower(strings.TrimSpace(arg))
	switch dateArg {
	case "today":
		return now, nil
	case "yesterday":
		return now.AddDate(0, 0, -1), nil
	case "tomorrow":
		return now.AddDate(0, 0, 1), nil
	default:
		parsed, err := ParseDate(dateArg)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date format '%s', use YYYY-MM-DD or today/yesterday/tomorrow", dateArg)
		}
		return parsed, nil
	}
}

