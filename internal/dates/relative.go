package dates

import (
	"strings"
	"time"
)

// RelativeDateKind describes whether a relative date keyword resolves to a single date.
type RelativeDateKind int

const (
	RelativeDateUnknown RelativeDateKind = iota
	RelativeDateInstant
)

// RelativeDateResolution is the resolved representation of a relative date keyword.
type RelativeDateResolution struct {
	Keyword string
	Kind    RelativeDateKind
	Date    time.Time
}

var relativeDateKeywords = map[string]struct{}{
	"today":     {},
	"tomorrow":  {},
	"yesterday": {},
}

// NormalizeRelativeDateKeyword normalizes and validates a relative date keyword.
// Returns the canonical keyword and true when valid.
func NormalizeRelativeDateKeyword(value string) (string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if _, ok := relativeDateKeywords[normalized]; !ok {
		return "", false
	}
	return normalized, true
}

// IsRelativeDateKeyword reports whether value is a supported relative date keyword.
func IsRelativeDateKeyword(value string) bool {
	_, ok := NormalizeRelativeDateKeyword(value)
	return ok
}

// ResolveRelativeDateKeyword resolves a relative date keyword using the provided "now".
func ResolveRelativeDateKeyword(value string, now time.Time, _ time.Weekday) (RelativeDateResolution, bool) {
	keyword, ok := NormalizeRelativeDateKeyword(value)
	if !ok {
		return RelativeDateResolution{}, false
	}

	anchor := startOfDay(now)
	switch keyword {
	case "today":
		return instantResolution(keyword, anchor), true
	case "tomorrow":
		return instantResolution(keyword, anchor.AddDate(0, 0, 1)), true
	case "yesterday":
		return instantResolution(keyword, anchor.AddDate(0, 0, -1)), true
	default:
		return RelativeDateResolution{}, false
	}
}

func startOfDay(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
}

func instantResolution(keyword string, date time.Time) RelativeDateResolution {
	return RelativeDateResolution{
		Keyword: keyword,
		Kind:    RelativeDateInstant,
		Date:    startOfDay(date),
	}
}
