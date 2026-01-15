package atomicfile

import (
	"fmt"
	"os"
	"path/filepath"
)

// WriteFile writes data to path atomically (best-effort cross-platform).
//
// It writes to a temporary file in the same directory and renames it into place.
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

	// On Windows, renaming over an existing file fails. Remove first (not atomic).
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(path)
		if err2 := os.Rename(tmpPath, path); err2 != nil {
			return fmt.Errorf("rename temp file: %w", err)
		}
	}

	committed = true
	return nil
}
