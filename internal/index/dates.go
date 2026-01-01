package index

import (
	"strings"
	"time"
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
	today := time.Now().Format("2006-01-02")

	switch filter {
	case "today":
		return fieldExpr + " = ?", []interface{}{today}, nil

	case "yesterday":
		yesterday := time.Now().AddDate(0, 0, -1).Format("2006-01-02")
		return fieldExpr + " = ?", []interface{}{yesterday}, nil

	case "tomorrow":
		tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
		return fieldExpr + " = ?", []interface{}{tomorrow}, nil

	case "this-week":
		now := time.Now()
		// Find start of week (Monday)
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7 // Sunday
		}
		startOfWeek := now.AddDate(0, 0, -(weekday - 1)).Format("2006-01-02")
		endOfWeek := now.AddDate(0, 0, 7-weekday).Format("2006-01-02")
		return fieldExpr + " >= ? AND " + fieldExpr + " <= ?",
			[]interface{}{startOfWeek, endOfWeek}, nil

	case "next-week":
		now := time.Now()
		weekday := int(now.Weekday())
		if weekday == 0 {
			weekday = 7
		}
		startOfNextWeek := now.AddDate(0, 0, 8-weekday).Format("2006-01-02")
		endOfNextWeek := now.AddDate(0, 0, 14-weekday).Format("2006-01-02")
		return fieldExpr + " >= ? AND " + fieldExpr + " <= ?",
			[]interface{}{startOfNextWeek, endOfNextWeek}, nil

	case "past":
		return fieldExpr + " < ? AND " + fieldExpr + " IS NOT NULL",
			[]interface{}{today}, nil

	case "future":
		return fieldExpr + " > ? AND " + fieldExpr + " IS NOT NULL",
			[]interface{}{today}, nil

	default:
		// Assume it's a YYYY-MM-DD date
		return fieldExpr + " = ?", []interface{}{filter}, nil
	}
}

// QueryByDate queries the date_index for items on a specific date or date range.
func (d *Database) QueryByDate(dateFilter string) ([]DateIndexResult, error) {
	condition, args, err := ParseDateFilter(dateFilter, "date")
	if err != nil {
		return nil, err
	}

	query := "SELECT date, source_type, source_id, field_name, file_path FROM date_index WHERE " + condition
	rows, err := d.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []DateIndexResult
	for rows.Next() {
		var result DateIndexResult
		if err := rows.Scan(&result.Date, &result.SourceType, &result.SourceID, &result.FieldName, &result.FilePath); err != nil {
			return nil, err
		}
		results = append(results, result)
	}

	return results, rows.Err()
}
