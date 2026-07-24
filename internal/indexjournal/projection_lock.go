package indexjournal

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/filelock"
)

const projectionLockFilename = "index-projection.lock"

// ProjectionLock serializes derived index projection and recovery. Callers
// acquire the SQLite index lock first, then this lock, to keep lock ordering
// consistent with full rebuilds.
type ProjectionLock struct {
	file *os.File
}

// LockProjection acquires the vault's projection lock.
func LockProjection(vaultPath string) (*ProjectionLock, error) {
	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		return nil, fmt.Errorf("create .raven directory: %w", err)
	}
	lockFile, err := os.OpenFile(filepath.Join(ravenDir, projectionLockFilename), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open index projection lock: %w", err)
	}
	if err := filelock.LockExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("lock index projection: %w", err)
	}
	return &ProjectionLock{file: lockFile}, nil
}

// Close releases the projection lock.
func (l *ProjectionLock) Close() error {
	if l == nil || l.file == nil {
		return nil
	}
	unlockErr := filelock.Unlock(l.file)
	closeErr := l.file.Close()
	l.file = nil
	return errors.Join(unlockErr, closeErr)
}
