package atomicfile

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
)

// WriteFile writes data to path atomically (best-effort cross-platform).
//
// It writes to a temporary file in the same directory, renames it into place,
// and syncs the parent directory (where supported) to persist the rename.
// This avoids torn writes if the process crashes mid-write.
//
// perm is used for the temp file. If perm is 0, WriteFile will try to preserve the
// existing file's mode (if it exists) and otherwise falls back to 0644.
func WriteFile(path string, data []byte, perm os.FileMode) error {
	if perm == 0 {
		if st, err := os.Stat(path); err == nil {
			perm = st.Mode()
		} else {
			perm = 0o644
		}
	}

	dir := filepath.Dir(path)
	base := filepath.Base(path)

	tmp, err := os.CreateTemp(dir, "."+base+".tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}

	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()

	// Best-effort; some platforms/filesystems may not support chmod here.
	_ = tmp.Chmod(perm)

	if _, err := tmp.Write(data); err != nil {
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmp.Sync(); err != nil {
		// Best-effort; still prefer returning the error as callers may care.
		return fmt.Errorf("sync temp file: %w", err)
	}

	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temp file: %w", err)
	}

	// On Windows, renaming over an existing file fails. Preserve the destination
	// while using a non-atomic backup/replace fallback.
	if err := os.Rename(tmpPath, path); err != nil {
		if runtime.GOOS != "windows" || !fileExists(path) {
			return fmt.Errorf("rename temp file: %w", err)
		}
		if replaceErr := replaceExistingFile(tmpPath, path, os.Rename, os.Remove); replaceErr != nil {
			return fmt.Errorf("rename temp file: %w", errors.Join(err, replaceErr))
		}
	}

	committed = true

	// Sync the parent directory to persist the rename on POSIX systems.
	// Skip on Windows where opening directories for sync is not supported.
	if runtime.GOOS != "windows" {
		if err := syncDir(dir); err != nil {
			// Ignore ENOTSUP for filesystems that don't support directory sync
			if !errors.Is(err, syscall.ENOTSUP) {
				return fmt.Errorf("sync directory: %w", err)
			}
		}
	}

	return nil
}

func syncDir(dir string) error {
	// Open the directory and call Sync() to flush its metadata (persisting the
	// rename). Only called on non-Windows systems.
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer d.Close()
	return d.Sync()
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func replaceExistingFile(
	tmpPath string,
	path string,
	rename func(string, string) error,
	remove func(string) error,
) error {
	backupPath := tmpPath + ".backup"
	if err := rename(path, backupPath); err != nil {
		return fmt.Errorf("back up destination: %w", err)
	}

	if err := rename(tmpPath, path); err != nil {
		if restoreErr := rename(backupPath, path); restoreErr != nil {
			return errors.Join(
				fmt.Errorf("replace destination: %w", err),
				fmt.Errorf("restore destination: %w", restoreErr),
			)
		}
		return fmt.Errorf("replace destination: %w", err)
	}

	_ = remove(backupPath)
	return nil
}
