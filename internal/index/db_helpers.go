package index

import (
	"database/sql"
	"fmt"
)

type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

// filePathTables is the single source of truth for indexed data tables keyed
// by file_path. Metadata is intentionally excluded.
var filePathTables = []string{"objects", "sections", "traits", "refs", "field_refs", "date_index", "fts_content", "assets"}

func deleteAllFilePathData(e execer) error {
	for _, table := range filePathTables {
		if _, err := e.Exec("DELETE FROM " + table); err != nil {
			return err
		}
	}
	return nil
}

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
