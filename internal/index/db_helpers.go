package index

import (
	"database/sql"
	"fmt"
)

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
