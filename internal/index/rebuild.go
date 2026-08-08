package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/filelock"
	"github.com/aidanlsb/raven/internal/indexschema"
)

// RebuildOptions configures an index rebuild session.
type RebuildOptions struct {
	// DryRun prevents the session from changing the on-disk index. An
	// incompatible index is represented by a temporary in-memory database.
	DryRun bool
}

// RebuildSession is the only path allowed to use an index while a rebuild is
// required. A schema wipe or full rebuild leaves a durable marker in place
// until Complete is called, so ordinary Open calls cannot observe partial data.
type RebuildSession struct {
	db            *Database
	lock          *indexLock
	markerPath    string
	schemaRebuilt bool
	markerPending bool
	dryRun        bool
	closed        bool
}

// OpenWithRebuild opens an index for reindexing. Incompatible or interrupted
// indexes are wiped and opened only inside a RebuildSession. The session must
// complete successfully before ordinary callers can open the rebuilt index.
func OpenWithRebuild(vaultPath string, opts RebuildOptions) (*RebuildSession, error) {
	dbDir := filepath.Join(vaultPath, ".raven")
	dbPath := filepath.Join(dbDir, "index.db")

	lock, err := acquireIndexLock(dbDir)
	if err != nil {
		return nil, err
	}
	releaseLock := true
	defer func() {
		if releaseLock {
			_ = lock.Release()
		}
	}()

	markerPath := filepath.Join(dbDir, rebuildRequiredFilename)
	markerExists, err := hasRebuildRequiredMarker(dbDir)
	if err != nil {
		return nil, err
	}
	incompatible, err := databaseNeedsRebuild(dbPath)
	if err != nil {
		return nil, err
	}
	rebuildRequired := markerExists || incompatible

	if rebuildRequired && opts.DryRun {
		db, err := OpenInMemory()
		if err != nil {
			return nil, err
		}
		releaseLock = false
		return &RebuildSession{
			db:            db,
			lock:          lock,
			markerPath:    markerPath,
			schemaRebuilt: true,
			dryRun:        true,
		}, nil
	}

	if rebuildRequired {
		if err := writeRebuildRequiredMarker(markerPath); err != nil {
			return nil, err
		}
		if err := removeDatabaseFiles(dbPath); err != nil {
			return nil, err
		}
	}

	db, err := openDatabase(vaultPath, true)
	if err != nil {
		return nil, err
	}

	releaseLock = false
	return &RebuildSession{
		db:            db,
		lock:          lock,
		markerPath:    markerPath,
		schemaRebuilt: rebuildRequired,
		markerPending: rebuildRequired,
		dryRun:        opts.DryRun,
	}, nil
}

// Database returns the database reserved for this rebuild session.
func (s *RebuildSession) Database() *Database {
	if s == nil {
		return nil
	}
	return s.db
}

// SchemaRebuilt reports whether opening this session had to replace an
// incompatible or interrupted index.
func (s *RebuildSession) SchemaRebuilt() bool {
	return s != nil && s.schemaRebuilt
}

// BeginFullRebuild marks the index unavailable before its existing data is
// cleared. The marker remains if the rebuild exits before Complete.
func (s *RebuildSession) BeginFullRebuild() error {
	if s == nil || s.dryRun || s.markerPending {
		return nil
	}
	if err := writeRebuildRequiredMarker(s.markerPath); err != nil {
		return err
	}
	s.markerPending = true
	return nil
}

// Complete makes a successfully rebuilt index available to ordinary callers.
func (s *RebuildSession) Complete() error {
	if s == nil || s.dryRun || !s.markerPending {
		return nil
	}
	if err := os.Remove(s.markerPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to clear index rebuild marker: %w", err)
	}
	s.markerPending = false
	return nil
}

// Close closes the database and releases the rebuild lock. It intentionally
// leaves any pending rebuild marker in place.
func (s *RebuildSession) Close() error {
	if s == nil || s.closed {
		return nil
	}
	s.closed = true

	var dbErr error
	if s.db != nil {
		dbErr = s.db.Close()
	}
	var lockErr error
	if s.lock != nil {
		lockErr = s.lock.Release()
	}
	return errors.Join(dbErr, lockErr)
}

type indexLock struct {
	file *os.File
}

func acquireIndexLock(dbDir string) (*indexLock, error) {
	return acquireIndexFileLock(dbDir, filelock.TryLockExclusive)
}

func acquireIndexReadLock(dbDir string) (*indexLock, error) {
	return acquireIndexFileLock(dbDir, filelock.TryLockShared)
}

func acquireIndexFileLock(dbDir string, tryLock func(*os.File) error) (*indexLock, error) {
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create .raven directory: %w", err)
	}

	lockPath := filepath.Join(dbDir, "index.lock")
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open index lock: %w", err)
	}

	if err := tryLock(lockFile); err != nil {
		lockFile.Close()
		if filelock.IsWouldBlock(err) {
			return nil, ErrIndexLocked
		}
		return nil, fmt.Errorf("failed to acquire index lock: %w", err)
	}

	return &indexLock{file: lockFile}, nil
}

func (l *indexLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := filelock.Unlock(l.file)
	closeErr := l.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}

func removeDatabaseFiles(dbPath string) error {
	paths := []string{dbPath, dbPath + "-wal", dbPath + "-shm"}
	for _, p := range paths {
		if err := os.Remove(p); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to remove %s: %w", p, err)
		}
	}
	return nil
}

func hasRebuildRequiredMarker(dbDir string) (bool, error) {
	_, err := os.Stat(filepath.Join(dbDir, rebuildRequiredFilename))
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("failed to inspect index rebuild marker: %w", err)
}

func writeRebuildRequiredMarker(markerPath string) error {
	content := fmt.Sprintf("full reindex required for database version %d\n", indexschema.CurrentDBVersion)
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write index rebuild marker: %w", err)
	}
	return nil
}

func databaseNeedsRebuild(dbPath string) (bool, error) {
	isNewDB, err := isNewDatabaseFile(dbPath)
	if err != nil {
		return false, err
	}
	if isNewDB {
		return false, nil
	}

	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return false, fmt.Errorf("failed to open database for schema check: %w", err)
	}
	defer db.Close()
	return !isSchemaCompatible(db), nil
}

// isSchemaCompatible checks if the database schema matches expected structure.
func isSchemaCompatible(db *sql.DB) bool {
	version, ok, err := storedDatabaseVersion(db)
	if err != nil || !ok {
		return false
	}
	return version == indexschema.CurrentDBVersion
}

// SchemaCompatible reports whether this database has the current index schema.
func (d *Database) SchemaCompatible() (bool, error) {
	version, ok, err := storedDatabaseVersion(d.db)
	if err != nil {
		return false, err
	}
	return ok && version == indexschema.CurrentDBVersion, nil
}
