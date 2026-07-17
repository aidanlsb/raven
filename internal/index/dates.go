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

// TryParseTemporalComparisonWithOptions parses date, relative-date, and datetime
// filter values and returns a SQLite temporal comparison. Date and relative-date
// inputs compare by calendar date; datetime inputs compare by datetime.
func TryParseTemporalComparisonWithOptions(filter string, op string, fieldExpr string, opts DateFilterOptions) (condition string, args []interface{}, ok bool, err error) {
	opts = normalizeDateFilterOptions(opts)
	input, isTemporal, err := dates.ParseInput(filter, opts.Now)
	if err != nil {
		return "", nil, false, fmt.Errorf("invalid date filter: %q", strings.TrimSpace(filter))
	}
	if !isTemporal {
		return "", nil, false, nil
	}

	switch op {
	case "=", "!=", "<", "<=", ">", ">=":
	default:
		return "", nil, false, fmt.Errorf("unsupported date comparison operator: %s", op)
	}

	if input.Kind == dates.InputDatetimeLiteral {
		return fmt.Sprintf("datetime(%s) %s datetime(?)", fieldExpr, op), []interface{}{input.Raw}, true, nil
	}
	return fmt.Sprintf("date(%s) %s date(?)", fieldExpr, op), []interface{}{input.CalendarDate}, true, nil
}

func normalizeDateFilterOptions(opts DateFilterOptions) DateFilterOptions {
	if opts.Now.IsZero() {
		opts.Now = time.Now()
	}
	return opts
}
