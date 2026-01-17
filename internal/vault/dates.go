// Package vault provides utilities for vault operations.
package vault

import (
	"time"

	"github.com/aidanlsb/raven/internal/dates"
)

// ParseDateArg parses a date argument which can be:
// - "today", "yesterday", "tomorrow" (relative dates)
// - "YYYY-MM-DD" format (absolute date)
// - Empty string defaults to today
func ParseDateArg(arg string) (time.Time, error) {
	// Preserve historical behavior: default to time.Now() when empty, and accept
	// today/yesterday/tomorrow or YYYY-MM-DD (case-insensitive).
	return dates.ParseDateArg(arg, time.Now())
}

// FormatDateISO formats a time as YYYY-MM-DD.
func FormatDateISO(t time.Time) string {
	return t.Format(dates.DateLayout)
}

// FormatDateFriendly formats a time as "Monday, January 2, 2006".
func FormatDateFriendly(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}
