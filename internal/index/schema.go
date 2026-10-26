package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/indexschema"
)

// initialize creates the database schema.
func (d *Database) initialize(isNewDB bool) error {
	if !isNewDB {
		compatible, err := d.SchemaCompatible()
		if err != nil {
			return fmt.Errorf("failed to read database version: %w", err)
		}
		if compatible {
			// CREATE TABLE/INDEX/TRIGGER/VIRTUAL TABLE IF NOT EXISTS on every
			// CLI open is dominated by FTS5 virtual-table setup. The version
			// row is the compatibility contract; skip that DDL when it matches.
			// Fingerprints are a side table added without a version bump, so
			// keep the cheap IF NOT EXISTS create.
			if _, err := d.db.Exec(fileFingerprintSQL); err != nil {
				return fmt.Errorf("failed to initialize file fingerprints: %w", err)
			}
			if err := ensureRevision(d.db, false); err != nil {
				return fmt.Errorf("failed to initialize index revision: %w", err)
			}
			return nil
		}
	}

	_, err := d.db.Exec(indexschema.SchemaSQL)
	if err != nil {
		return fmt.Errorf("failed to initialize database schema: %w", err)
	}

	if _, err := d.db.Exec(fileFingerprintSQL); err != nil {
		return fmt.Errorf("failed to initialize file fingerprints: %w", err)
	}
	if err := ensureRevision(d.db, isNewDB); err != nil {
		return fmt.Errorf("failed to initialize index revision: %w", err)
	}

	if isNewDB {
		if err := setDatabaseVersion(d.db, indexschema.CurrentDBVersion); err != nil {
			return fmt.Errorf("failed to set database version: %w", err)
		}
	}

	return nil
}

func isNewDatabaseFile(dbPath string) (bool, error) {
	info, err := os.Stat(dbPath)
	if err == nil {
		return info.Size() == 0, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return true, nil
	}
	return false, fmt.Errorf("failed to inspect database file: %w", err)
}

func storedDatabaseVersion(db *sql.DB) (int, bool, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM meta WHERE key = 'version'`).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}

	version, ok := parseDatabaseVersion(raw)
	if !ok {
		return 0, false, nil
	}
	return version, true, nil
}

func parseDatabaseVersion(raw string) (int, bool) {
	version, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return version, true
}

func setDatabaseVersion(db *sql.DB, version int) error {
	_, err := db.Exec(`INSERT OR REPLACE INTO meta (key, value) VALUES ('version', ?)`, strconv.Itoa(version))
	return err
}
