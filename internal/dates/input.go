package dates

import (
	"strings"
	"time"
)

// InputKind identifies the source form used for a date-like input.
type InputKind int

const (
	InputUnknown InputKind = iota
	InputDateLiteral
	InputRelativeDate
	InputDatetimeLiteral
)

// Input is the canonical parsed representation for date and datetime inputs.
//
// CalendarDate is always YYYY-MM-DD. For datetime inputs, CalendarDate is the
// date component used by date-oriented indexes and daily-note references, while
// Datetime preserves the parsed instant for temporal comparisons.
type Input struct {
	Kind         InputKind
	Raw          string
	Keyword      string
	CalendarDate string
	Datetime     time.Time
}

// ParseInput parses absolute dates, relative date keywords, and datetimes.
func ParseInput(raw string, now time.Time) (Input, bool, error) {
	normalized := strings.TrimSpace(raw)
	if normalized == "" {
		return Input{}, false, nil
	}

	if IsValidDate(normalized) {
		return Input{
			Kind:         InputDateLiteral,
			Raw:          normalized,
			CalendarDate: normalized,
		}, true, nil
	}

	relative, ok := ResolveRelativeDateKeyword(normalized, now, time.Monday)
	if ok && relative.Kind == RelativeDateInstant {
		return Input{
			Kind:         InputRelativeDate,
			Raw:          normalized,
			Keyword:      relative.Keyword,
			CalendarDate: relative.Date.Format(DateLayout),
			Datetime:     relative.Date,
		}, true, nil
	}

	if IsValidDatetime(normalized) {
		parsed, err := ParseDatetime(normalized)
		if err != nil {
			return Input{}, false, err
		}
		return Input{
			Kind:         InputDatetimeLiteral,
			Raw:          normalized,
			CalendarDate: parsed.Format(DateLayout),
			Datetime:     parsed,
		}, true, nil
	}

	if LooksLikeDatetimeLiteral(normalized) {
		_, err := ParseDatetime(normalized)
		return Input{}, false, err
	}
	if LooksLikeDateLiteral(normalized) {
		_, err := ParseDate(normalized)
		return Input{}, false, err
	}

	return Input{}, false, nil
}

// CalendarDateForInput resolves a date or relative-date input to YYYY-MM-DD.
func CalendarDateForInput(raw string, now time.Time) (string, bool, error) {
	input, ok, err := ParseInput(raw, now)
	if err != nil || !ok {
		return "", ok, err
	}
	if input.Kind != InputDateLiteral && input.Kind != InputRelativeDate {
		return "", false, nil
	}
	return input.CalendarDate, true, nil
}

// DailyObjectID returns the object ID for a calendar date.
//
// The canonical daily-note object ID is the bare ISO date (YYYY-MM-DD). The
// daily directory is filesystem layout only and is not part of the identity.
func DailyObjectID(calendarDate string) string {
	return calendarDate
}

// DailyObjectIDForInput resolves a date input and returns its daily-note object ID.
func DailyObjectIDForInput(raw string, now time.Time) (string, bool, error) {
	calendarDate, ok, err := CalendarDateForInput(raw, now)
	if err != nil || !ok {
		return "", ok, err
	}
	return DailyObjectID(calendarDate), true, nil
}

// LooksLikeDateLiteral reports whether value has the YYYY-MM-DD shape.
func LooksLikeDateLiteral(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) != len(DateLayout) {
		return false
	}
	return value[4] == '-' && value[7] == '-'
}

// LooksLikeDatetimeLiteral reports whether value starts with the supported
// YYYY-MM-DDT datetime shape.
func LooksLikeDatetimeLiteral(value string) bool {
	value = strings.TrimSpace(value)
	if len(value) < len(DatetimeLayout) {
		return false
	}
	return LooksLikeDateLiteral(value[:len(DateLayout)]) && value[len(DateLayout)] == 'T'
}
