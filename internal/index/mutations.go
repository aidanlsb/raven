package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RemoveFile removes all data for a file.
func (d *Database) RemoveFile(filePath string) error {
	return d.RemoveFiles([]string{filePath})
}

// RemoveFiles removes all indexed data for one or more file paths in a single transaction.
func (d *Database) RemoveFiles(filePaths []string) error {
	if len(filePaths) == 0 {
		return nil
	}
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.ensureReferenceResolverCacheCurrentLocked(tx); err != nil {
		return err
	}
	oldResolverStates := make([]*resolverFileState, 0, len(filePaths))
	for _, filePath := range filePaths {
		state, err := d.cachedResolverFileStateLocked(tx, filePath)
		if err != nil {
			return err
		}
		oldResolverStates = append(oldResolverStates, state)
	}
	for _, filePath := range filePaths {
		if err := deleteByFilePath(tx, filePath); err != nil {
			return err
		}
	}

	resolverGeneration, err := bumpResolverGeneration(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if !d.autoResolveRefs {
		d.referenceResolverCache = nil
		return nil
	}
	for _, oldState := range oldResolverStates {
		d.updateReferenceResolverCacheLocked(oldState, nil)
	}
	d.setReferenceResolverGenerationLocked(resolverGeneration)
	return nil
}

// ClearAllData removes all indexed data from the database.
// This is used for full reindex to ensure a clean slate.
func (d *Database) ClearAllData() error {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := deleteAllFilePathData(tx); err != nil {
		return err
	}

	if _, err := bumpResolverGeneration(tx); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	d.referenceResolverCache = nil
	return nil
}

// RemoveFilesWithPrefix removes all data for files whose paths start with a given prefix.
// This is used to clean up files in excluded directories like .trash/.
// Returns the number of files removed.
func (d *Database) RemoveFilesWithPrefix(pathPrefix string) (int, error) {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	tx, err := d.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	// Count files that will be removed
	count, err := countDistinctFilesWithPrefix(tx, pathPrefix)
	if err != nil {
		return 0, err
	}

	if count == 0 {
		return 0, nil
	}

	pattern := pathPrefix + "%"
	if err := deleteByFilePathLike(tx, pattern); err != nil {
		return 0, err
	}
	if _, err := bumpResolverGeneration(tx); err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	d.referenceResolverCache = nil
	return count, nil
}

func countDistinctFilesWithPrefix(db resolverQuerier, pathPrefix string) (int, error) {
	var count int
	err := db.QueryRow(`
		SELECT COUNT(DISTINCT file_path)
		FROM (
			SELECT file_path FROM objects
			UNION
			SELECT file_path FROM assets
		)
		WHERE file_path LIKE ?
	`, pathPrefix+"%").Scan(&count)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// AllIndexedFilePaths returns all distinct file paths currently in the index.
// This is useful for detecting deleted files during incremental reindexing.
func (d *Database) AllIndexedFilePaths() ([]string, error) {
	rows, err := d.db.Query(`
		SELECT DISTINCT file_path FROM objects
		UNION
		SELECT DISTINCT file_path FROM assets
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		paths = append(paths, path)
	}
	return paths, rows.Err()
}

// RemoveDeletedFiles removes index entries for files that no longer exist on the filesystem.
// Returns the list of removed file paths.
func (d *Database) RemoveDeletedFiles(vaultPath string) ([]string, error) {
	indexedPaths, err := d.AllIndexedFilePaths()
	if err != nil {
		return nil, fmt.Errorf("failed to get indexed paths: %w", err)
	}

	var removed []string
	for _, relPath := range indexedPaths {
		if fileMissing(filepath.Join(vaultPath, relPath)) {
			removed = append(removed, relPath)
		}
	}
	if err := d.RemoveFiles(removed); err != nil {
		return nil, fmt.Errorf("failed to remove deleted files: %w", err)
	}

	return removed, nil
}

func fileMissing(fullPath string) bool {
	_, err := os.Stat(fullPath)
	return os.IsNotExist(err)
}

// RemoveDocument removes a document and all related data by its object ID.
func (d *Database) RemoveDocument(objectID string) error {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	// Objects can have IDs like "people/freya" or "daily/2025-02-01#meeting".
	// This method removes the *entire file/document* from the index.
	//
	// Callers may pass a section ID (with a '#'). In that case we still
	// remove the whole document, since sections are derived from
	// the markdown file without rewriting content.
	baseID := baseDocumentID(objectID)

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := d.ensureReferenceResolverCacheCurrentLocked(tx); err != nil {
		return err
	}
	// Prefer the canonical file_path stored in the DB (important when directory
	// roots are configured and object IDs do not match file paths).
	var filePath string
	err = tx.QueryRow(
		"SELECT file_path FROM objects WHERE id = ? OR id LIKE ? LIMIT 1",
		baseID,
		baseID+"#%",
	).Scan(&filePath)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrObjectNotFound
		} else {
			return err
		}
	}

	oldResolverState, err := d.cachedResolverFileStateLocked(tx, filePath)
	if err != nil {
		return err
	}
	if err := deleteByFilePath(tx, filePath); err != nil {
		return err
	}
	resolverGeneration, err := bumpResolverGeneration(tx)
	if err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if d.autoResolveRefs {
		d.updateReferenceResolverCacheLocked(oldResolverState, nil)
		d.setReferenceResolverGenerationLocked(resolverGeneration)
	} else {
		d.referenceResolverCache = nil
	}
	return nil
}

func baseDocumentID(objectID string) string {
	baseID := objectID
	if hash := strings.Index(baseID, "#"); hash >= 0 {
		baseID = baseID[:hash]
	}
	return baseID
}
