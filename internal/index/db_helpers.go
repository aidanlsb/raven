package index

import (
	"database/sql"
	"fmt"
	"strings"
)

// inClauseArgs returns a comma-separated list of "?" placeholders and the
// corresponding args slice. If items is empty, returns "NULL" and no args.
func inClauseArgs(items []string) (placeholders string, args []any) {
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

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

var filePathTables = []string{"objects", "traits", "refs", "field_refs", "date_index", "fts_content"}

func deleteByFilePath(e execer, filePath string) error {
	for _, table := range filePathTables {
		if _, err := e.Exec("DELETE FROM "+table+" WHERE file_path = ?", filePath); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}
	return nil
}

func deleteByFilePathLike(e execer, likePattern string) error {
	for _, table := range filePathTables {
		if _, err := e.Exec("DELETE FROM "+table+" WHERE file_path LIKE ?", likePattern); err != nil {
			return fmt.Errorf("delete from %s: %w", table, err)
		}
	}
	return nil
}
