package objectsvc

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/filelock"
)

// appendLocksDir is the .raven-relative directory containing per-target append
// lock files. Coordination happens through sidecar files (not the destination
// file itself) so the lock survives the rename performed by atomicfile.WriteFile
// during read-modify-write append paths.
const appendLocksDir = "append-locks"

// acquireAppendLock returns a release function that must be invoked to unlock
// and close the coordinating lock file. The lock is exclusive and serializes
// concurrent appends to the same destination across goroutines and processes.
//
// When vaultPath is known, the lock lives under <vault>/.raven/append-locks/
// keyed by a hash of the destination's vault-relative path. When the
// destination is outside the vault (or vaultPath is empty), we place a hidden
// sidecar lock file next to the destination. In both cases the lock file is a
// separate inode from the destination itself, so it survives the rename
// performed by atomicfile.WriteFile and composes safely with any additional
// flock taken on the destination file descriptor by callers.
func acquireAppendLock(vaultPath, destPath string) (release func(), err error) {
	lockPath, err := appendLockFilePath(vaultPath, destPath)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return nil, fmt.Errorf("create append lock directory: %w", err)
	}
	lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, fmt.Errorf("open append lock: %w", err)
	}
	if err := filelock.LockExclusive(lockFile); err != nil {
		_ = lockFile.Close()
		return nil, fmt.Errorf("acquire append lock: %w", err)
	}
	return func() {
		_ = filelock.Unlock(lockFile)
		_ = lockFile.Close()
	}, nil
}

// appendLockFilePath returns the path to the sidecar lock file coordinating
// appends to destPath. When possible the lock lives under the vault's .raven
// directory; otherwise it is placed next to the destination.
func appendLockFilePath(vaultPath, destPath string) (string, error) {
	if p, ok := sidecarLockPath(vaultPath, destPath); ok {
		return p, nil
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return "", fmt.Errorf("resolve destination for append lock: %w", err)
	}
	dir := filepath.Dir(absDest)
	base := filepath.Base(absDest)
	return filepath.Join(dir, "."+base+".rvn-append.lock"), nil
}

// sidecarLockPath returns the vault-scoped sidecar lock file path for destPath
// and whether the vault-scoped strategy applies. When the destination is not
// inside the vault (or vaultPath is empty), returns ok=false so callers fall
// back to a destination-local sidecar.
func sidecarLockPath(vaultPath, destPath string) (string, bool) {
	if vaultPath == "" {
		return "", false
	}
	absVault, err := filepath.Abs(vaultPath)
	if err != nil {
		return "", false
	}
	absDest, err := filepath.Abs(destPath)
	if err != nil {
		return "", false
	}
	relPath, err := filepath.Rel(absVault, absDest)
	if err != nil {
		return "", false
	}
	relPath = filepath.ToSlash(relPath)
	if relPath == "" || strings.HasPrefix(relPath, "../") || relPath == ".." {
		return "", false
	}
	sum := sha256.Sum256([]byte(relPath))
	name := hex.EncodeToString(sum[:]) + ".lock"
	return filepath.Join(absVault, ".raven", appendLocksDir, name), true
}
