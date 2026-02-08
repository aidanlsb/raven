package index

import (
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
)

// ParseDateFilter parses a date filter string and returns the SQL condition and args.
// Supports:
//   - "today"
//   - "yesterday"
//   - "tomorrow"
//   - "this-week"
//   - "next-week"
//   - "past" (before today)
//   - "future" (after today)
//   - "YYYY-MM-DD" (exact date)
func ParseDateFilter(filter string, fieldExpr string) (condition string, args []interface{}, err error) {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return "", nil, fmt.Errorf("invalid date filter: %q", filter)
	}

	now := time.Now()
	today := now.Format(dates.DateLayout)

	switch filter {
	case "this-week":
		// Find start of week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday
		}
		startOfWeek := now.AddDate(0, 0, -(weekday - 1)).Format(dates.DateLayout)
		endOfWeek := now.AddDate(0, 0, 7-weekday).Format(dates.DateLayout)
		return fieldExpr + " >= ? AND " + fieldExpr + " <= ?",
			[]interface{}{startOfWeek, endOfWeek}, nil

	case "next-week":
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfNextWeek := now.AddDate(0, 0, 8-weekday).Format(dates.DateLayout)
		endOfNextWeek := now.AddDate(0, 0, 14-weekday).Format(dates.DateLayout)
		return fieldExpr + " >= ? AND " + fieldExpr + " <= ?",
			[]interface{}{startOfNextWeek, endOfNextWeek}, nil

	case "past":
		return fieldExpr + " < ? AND " + fieldExpr + " IS NOT NULL",
			[]interface{}{today}, nil

	case "future":
		return fieldExpr + " > ? AND " + fieldExpr + " IS NOT NULL",
			[]interface{}{today}, nil

	default:
		parsed, parseErr := dates.ParseDateArg(filter, now)
		if parseErr != nil {
			return "", nil, fmt.Errorf("invalid date filter: %q", filter)
		}
		return fieldExpr + " = ?", []interface{}{parsed.Format(dates.DateLayout)}, nil
	}
}
