// Package index handles SQLite database operations.
package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	_ "modernc.org/sqlite"
)

// Database is the SQLite database handle.
type Database struct {
	db                      *sql.DB
	dailyDirectory          string
	autoResolveRefs         bool
	lock                    *indexLock
	resolverMu              sync.Mutex
	referenceResolverCache  *referenceResolverCache
	referenceResolverBuilds int
}

var (
	// ErrObjectNotFound indicates the requested object ID is not in the index.
	ErrObjectNotFound = errors.New("object not found in index")
	// ErrIndexLocked indicates another process is rebuilding the index.
	ErrIndexLocked = errors.New("index is locked for rebuild")
	// ErrIndexRebuildRequired indicates the index cannot be used until a full
	// reindex completes.
	ErrIndexRebuildRequired = errors.New("index requires a full reindex")
)

const rebuildRequiredFilename = "reindex-required"

const sqliteBusyTimeoutMillis = 5000

// DB returns the underlying sql.DB for advanced queries.
func (d *Database) DB() *sql.DB {
	return d.db
}

// Open opens or creates the database.
func Open(vaultPath string) (*Database, error) {
	dbDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .raven directory: %w", err)
	}

	lock, err := acquireIndexReadLock(dbDir)
	if err != nil {
		return nil, err
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			_ = lock.Release()
		}
	}()

	rebuildRequired, err := hasRebuildRequiredMarker(dbDir)
	if err != nil {
		return nil, err
	}
	if rebuildRequired {
		return nil, ErrIndexRebuildRequired
	}

	db, err := openDatabase(vaultPath, false)
	if err != nil {
		return nil, err
	}
	db.lock = lock
	releaseLock = false
	return db, nil
}

func openDatabase(vaultPath string, allowIncompatible bool) (*Database, error) {
	dbDir := filepath.Join(vaultPath, ".raven")
	dbPath := filepath.Join(dbDir, "index.db")
	isNewDB, err := isNewDatabaseFile(dbPath)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}
	if err := configureDatabaseConnection(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	if !isNewDB && !allowIncompatible && !isSchemaCompatible(db) {
		_ = db.Close()
		return nil, ErrIndexRebuildRequired
	}

	d := &Database{db: db, dailyDirectory: "daily", autoResolveRefs: true}
	if err := d.initialize(isNewDB); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

// OpenInMemory opens an in-memory database (for testing).
func OpenInMemory() (*Database, error) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, err
	}
	if err := configureDatabaseConnection(db); err != nil {
		_ = db.Close()
		return nil, err
	}

	d := &Database{db: db, dailyDirectory: "daily", autoResolveRefs: true}
	if err := d.initialize(true); err != nil {
		db.Close()
		return nil, err
	}

	return d, nil
}

func configureDatabaseConnection(db *sql.DB) error {
	// SQLite permits one writer at a time. Keep each Raven handle on one
	// connection so connection-local PRAGMAs are stable, then wait briefly for
	// concurrent WAL writers instead of immediately returning SQLITE_BUSY.
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec(fmt.Sprintf("PRAGMA busy_timeout = %d", sqliteBusyTimeoutMillis)); err != nil {
		return fmt.Errorf("failed to configure database busy timeout: %w", err)
	}
	return nil
}

// Close closes the database.
func (d *Database) Close() error {
	if d == nil {
		return nil
	}
	var dbErr error
	if d.db != nil {
		dbErr = d.db.Close()
	}
	var lockErr error
	if d.lock != nil {
		lockErr = d.lock.Release()
		d.lock = nil
	}
	return errors.Join(dbErr, lockErr)
}

// SetDailyDirectory configures the daily notes directory for reference resolution.
func (d *Database) SetDailyDirectory(dailyDir string) {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	if dailyDir == "" {
		dailyDir = "daily"
	}
	if d.dailyDirectory != dailyDir {
		d.referenceResolverCache = nil
	}
	d.dailyDirectory = dailyDir
}

// SetAutoResolveRefs toggles resolve-on-write behavior.
func (d *Database) SetAutoResolveRefs(enabled bool) {
	d.resolverMu.Lock()
	defer d.resolverMu.Unlock()

	if d.autoResolveRefs != enabled {
		d.referenceResolverCache = nil
	}
	d.autoResolveRefs = enabled
}

// Analyze runs SQLite's ANALYZE command to update query planner statistics.
// This should be called after bulk indexing operations for optimal query performance.
func (d *Database) Analyze() error {
	_, err := d.db.Exec("ANALYZE")
	return err
}
