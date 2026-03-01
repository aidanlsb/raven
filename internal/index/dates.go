package index

import (
	"fmt"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/dates"
)

// DateFilterOptions controls relative-date resolution behavior.
type DateFilterOptions struct {
	Now time.Time
}

// ParseDateFilter parses a date filter string and returns the SQL condition and args
// for equality semantics.
func ParseDateFilter(filter string, fieldExpr string) (condition string, args []interface{}, err error) {
	return ParseDateFilterWithOptions(filter, fieldExpr, DateFilterOptions{
		Now: time.Now(),
	})
}

// ParseDateFilterWithOptions parses a date filter string and returns the SQL condition
// and args for equality semantics.
func ParseDateFilterWithOptions(filter string, fieldExpr string, opts DateFilterOptions) (condition string, args []interface{}, err error) {
	condition, args, ok, err := TryParseDateComparisonWithOptions(filter, "=", fieldExpr, opts)
	if err != nil {
		return "", nil, err
	}
	if !ok {
		return "", nil, fmt.Errorf("invalid date filter: %q", strings.TrimSpace(filter))
	}
	return condition, args, nil
}

// TryParseDateComparisonWithOptions parses a filter value for a specific comparison
// operator and returns:
// - condition + args when ok=true
// - ok=false when value is not a date keyword/date literal
// - err when value looks like a date input but is invalid
func TryParseDateComparisonWithOptions(filter string, op string, fieldExpr string, opts DateFilterOptions) (condition string, args []interface{}, ok bool, err error) {
	dateValue, isDate, err := resolveDateFilterValue(filter, opts)
	if err != nil {
		return "", nil, false, err
	}
	if !isDate {
		return "", nil, false, nil
	}

	switch op {
	case "=", "!=", "<", "<=", ">", ">=":
		return fieldExpr + " " + op + " ?", []interface{}{dateValue}, true, nil
	default:
		return "", nil, false, fmt.Errorf("unsupported date comparison operator: %s", op)
	}
}

func normalizeDateFilterOptions(opts DateFilterOptions) DateFilterOptions {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	return opts
}

func resolveDateFilterValue(filter string, opts DateFilterOptions) (string, bool, error) {
	normalized := strings.TrimSpace(filter)
	if normalized == "" {
		return "", false, fmt.Errorf("invalid date filter: %q", normalized)
	}

	opts = normalizeDateFilterOptions(opts)

	if dates.IsValidDate(normalized) {
		return normalized, true, nil
	}

	relative, ok := dates.ResolveRelativeDateKeyword(normalized, opts.Now, time.Monday)
	if ok && relative.Kind == dates.RelativeDateInstant {
		return relative.Date.Format(dates.DateLayout), true, nil
	}

	// If value looks like a date literal but isn't valid, surface an explicit error.
	if looksLikeDateLiteral(normalized) {
		return "", false, fmt.Errorf("invalid date filter: %q", normalized)
	}

	return "", false, nil
}

func looksLikeDateLiteral(value string) bool {
	if len(value) != 10 {
		return false
	}
	return value[4] == '-' && value[7] == '-'
}
