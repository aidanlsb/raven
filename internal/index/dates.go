package index

import "github.com/aidanlsb/raven/internal/indexschema"

// DateFilterOptions is retained as an index-package compatibility alias.
type DateFilterOptions = indexfieldvalue.DateFilterOptions

// TryParseTemporalComparisonWithOptions parses date, relative-date, and datetime
// filter values and returns a SQLite temporal comparison. Date and relative-date
// inputs compare by calendar date; datetime inputs compare by datetime.
func TryParseTemporalComparisonWithOptions(filter string, op string, fieldExpr string, opts DateFilterOptions) (condition string, args []interface{}, ok bool, err error) {
	return indexschema.TryParseTemporalComparisonWithOptions(filter, op, fieldExpr, opts)
}
