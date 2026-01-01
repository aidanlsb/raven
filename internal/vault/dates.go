// Package vault provides utilities for vault operations.
package vault

import (
	"fmt"
	"strings"
	"time"
)

// ParseDateArg parses a date argument which can be:
// - "today", "yesterday", "tomorrow" (relative dates)
// - "YYYY-MM-DD" format (absolute date)
// - Empty string defaults to today
func ParseDateArg(arg string) (time.Time, error) {
	if arg == "" {
		return time.Now(), nil
	}

	dateArg := strings.ToLower(strings.TrimSpace(arg))
	switch dateArg {
	case "today":
		return time.Now(), nil
	case "yesterday":
		return time.Now().AddDate(0, 0, -1), nil
	case "tomorrow":
		return time.Now().AddDate(0, 0, 1), nil
	default:
		parsed, err := time.Parse("2006-01-02", dateArg)
		if err != nil {
			return time.Time{}, fmt.Errorf("invalid date format '%s', use YYYY-MM-DD or today/yesterday/tomorrow", dateArg)
		}
		return parsed, nil
	}
}

// FormatDateISO formats a time as YYYY-MM-DD.
func FormatDateISO(t time.Time) string {
	return t.Format("2006-01-02")
}

// FormatDateFriendly formats a time as "Monday, January 2, 2006".
func FormatDateFriendly(t time.Time) string {
	return t.Format("Monday, January 2, 2006")
}
