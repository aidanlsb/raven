package index

import (
	"database/sql"
	"os"
	"path/filepath"
)

// StalenessInfo contains information about index freshness.
type StalenessInfo struct {
	IsStale      bool     // True if any files are stale
	StaleFiles   []string // List of stale file paths (relative to vault)
	TotalFiles   int      // Total number of indexed files
	CheckedFiles int      // Number of files checked
}

// CheckStaleness compares indexed file mtimes against current filesystem mtimes.
// vaultPath is needed to stat files. Returns info about which files are stale.
func (d *Database) CheckStaleness(vaultPath string) (*StalenessInfo, error) {
	info := &StalenessInfo{}

	// Get all unique file paths and their indexed mtimes
	rows, err := stalenessRows(d.db)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		filePath, indexedMtime, err := scanStalenessRow(rows)
		if err != nil {
			return nil, err
		}

		info.TotalFiles++

		stale, checked, err := isFileStaleAgainstIndexedMtime(filepath.Join(vaultPath, filePath), indexedMtime)
		if err != nil {
			// File was deleted or moved - consider stale
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
			continue
		}
		if checked {
			info.CheckedFiles++
		}
		if stale {
			info.StaleFiles = append(info.StaleFiles, filePath)
			info.IsStale = true
		}
	}

	return info, rows.Err()
}

func stalenessRows(db *sql.DB) (*sql.Rows, error) {
	return db.Query(`
		SELECT DISTINCT file_path, file_mtime
		FROM objects
		UNION
		SELECT DISTINCT file_path, file_mtime
		FROM assets
	`)
}

func scanStalenessRow(rows *sql.Rows) (string, sql.NullInt64, error) {
	var filePath string
	var indexedMtime sql.NullInt64
	if err := rows.Scan(&filePath, &indexedMtime); err != nil {
		return "", sql.NullInt64{}, err
	}
	return filePath, indexedMtime, nil
}

// isFileStaleAgainstIndexedMtime compares the current filesystem mtime to the indexed one.
//
// Returns:
// - stale: whether file should be considered stale (including missing indexed mtime)
// - checked: whether the file existed on disk (i.e., mtime was checked)
// - err: non-nil when os.Stat fails (caller decides how to treat)
func isFileStaleAgainstIndexedMtime(fullPath string, indexedMtime sql.NullInt64) (stale bool, checked bool, err error) {
	stat, err := os.Stat(fullPath)
	if err != nil {
		return false, false, err
	}
	checked = true
	currentMtime := stat.ModTime().Unix()
	// If no indexed mtime or current > indexed, file is stale
	if !indexedMtime.Valid || currentMtime > indexedMtime.Int64 {
		return true, checked, nil
	}
	return false, checked, nil
}

// GetFileMtime returns the indexed mtime for a file, or 0 if not found.
func (d *Database) GetFileMtime(filePath string) (int64, error) {
	var mtime sql.NullInt64
	err := d.db.QueryRow(`
		SELECT file_mtime
		FROM (
			SELECT file_mtime FROM objects WHERE file_path = ?
			UNION ALL
			SELECT file_mtime FROM assets WHERE file_path = ?
		)
		LIMIT 1
	`, filePath, filePath).Scan(&mtime)

	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	if !mtime.Valid {
		return 0, nil
	}
	return mtime.Int64, nil
}

// GetFileMtimes returns the indexed mtime for every indexed file.
// Files without an indexed mtime are included with a zero value.
func (d *Database) GetFileMtimes() (map[string]int64, error) {
	rows, err := d.db.Query(`
		SELECT file_path, MAX(file_mtime)
		FROM (
			SELECT file_path, file_mtime FROM objects
			UNION ALL
			SELECT file_path, file_mtime FROM assets
		)
		GROUP BY file_path
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	mtimes := make(map[string]int64)
	for rows.Next() {
		var filePath string
		var mtime sql.NullInt64
		if err := rows.Scan(&filePath, &mtime); err != nil {
			return nil, err
		}
		if mtime.Valid {
			mtimes[filePath] = mtime.Int64
		} else {
			mtimes[filePath] = 0
		}
	}
	return mtimes, rows.Err()
}
