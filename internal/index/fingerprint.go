package index

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const fileFingerprintSQL = `
	CREATE TABLE IF NOT EXISTS file_fingerprints (
		file_path TEXT PRIMARY KEY,
		file_mtime INTEGER NOT NULL,
		content_hash TEXT NOT NULL
	);
`

// FileFingerprint is the cheap identity used to skip redundant projection.
type FileFingerprint struct {
	Mtime int64
	Hash  string
}

// FileProjectionAction says whether a vault file needs to be indexed, removed,
// or is already current.
type FileProjectionAction int

const (
	// FileProjectionSkip means the indexed fingerprint already matches disk.
	FileProjectionSkip FileProjectionAction = iota
	// FileProjectionIndex means the file exists and its contents are not current.
	FileProjectionIndex
	// FileProjectionRemove means the file is gone and should leave the index.
	FileProjectionRemove
)

// FileContentHash returns the hex SHA-256 of file bytes.
func FileContentHash(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// FileFingerprint returns the stored mtime/hash for a vault-relative path.
func (d *Database) FileFingerprint(filePath string) (FileFingerprint, bool, error) {
	if d == nil || d.db == nil {
		return FileFingerprint{}, false, fmt.Errorf("database is nil")
	}
	var fp FileFingerprint
	err := d.db.QueryRow(
		`SELECT file_mtime, content_hash FROM file_fingerprints WHERE file_path = ?`,
		filePath,
	).Scan(&fp.Mtime, &fp.Hash)
	if errors.Is(err, sql.ErrNoRows) {
		return FileFingerprint{}, false, nil
	}
	if err != nil {
		return FileFingerprint{}, false, err
	}
	return fp, true, nil
}

// FileProjectionAction compares disk state to the stored fingerprint.
// Matching mtime skips the read. Matching content hash skips a rewrite
// when mtime moved (touch, atomic replace of identical bytes).
func (d *Database) FileProjectionAction(vaultPath, relPath string) (FileProjectionAction, error) {
	if d == nil {
		return FileProjectionIndex, fmt.Errorf("database is nil")
	}
	fullPath := filepath.Join(vaultPath, filepath.FromSlash(relPath))
	stat, err := os.Stat(fullPath)
	if os.IsNotExist(err) {
		return FileProjectionRemove, nil
	}
	if err != nil {
		return FileProjectionIndex, err
	}

	fp, ok, err := d.FileFingerprint(relPath)
	if err != nil {
		return FileProjectionIndex, err
	}
	fileMtime := stat.ModTime().Unix()
	if ok && fp.Mtime == fileMtime {
		return FileProjectionSkip, nil
	}
	content, err := os.ReadFile(fullPath)
	if err != nil {
		return FileProjectionIndex, err
	}
	if ok && fp.Hash == FileContentHash(content) {
		return FileProjectionSkip, nil
	}
	return FileProjectionIndex, nil
}

func upsertFileFingerprint(e execer, filePath string, mtime int64, hash string) error {
	_, err := e.Exec(
		`INSERT OR REPLACE INTO file_fingerprints (file_path, file_mtime, content_hash) VALUES (?, ?, ?)`,
		filePath,
		mtime,
		hash,
	)
	if err != nil {
		return fmt.Errorf("upsert file fingerprint: %w", err)
	}
	return nil
}
