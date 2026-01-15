package sqlutil

import (
	"database/sql"
	"strings"
)

// InClauseArgs returns a comma-separated list of "?" placeholders and the
// corresponding args slice.
//
// If items is empty, it returns "NULL" and no args, so `IN (NULL)` matches nothing.
func InClauseArgs(items []string) (placeholders string, args []any) {
	if len(items) == 0 {
		return "NULL", nil
	}
	ph := make([]string, len(items))
	args = make([]any, len(items))
	for i, item := range items {
		ph[i] = "?"
		args[i] = item
	}
	return strings.Join(ph, ", "), args
}

// ScanRows scans all rows into a slice using the provided scanner.
func ScanRows[T any](rows *sql.Rows, scan func(*sql.Rows) (T, error)) ([]T, error) {
	defer rows.Close()

	var out []T
	for rows.Next() {
		item, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}
