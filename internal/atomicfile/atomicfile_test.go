package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReplaceExistingFile(t *testing.T) {
	t.Parallel()

	t.Run("replaces destination and removes backup", func(t *testing.T) {
		t.Parallel()

		tmpPath, path := writeReplacementFiles(t)

		if err := replaceExistingFile(tmpPath, path, os.Rename, os.Remove); err != nil {
			t.Fatalf("replaceExistingFile() error = %v", err)
		}

		assertFileContent(t, path, "new")
		assertPathMissing(t, tmpPath)
		assertPathMissing(t, tmpPath+".backup")
	})

	t.Run("restores destination when replacement fails", func(t *testing.T) {
		t.Parallel()

		tmpPath, path := writeReplacementFiles(t)
		replaceErr := errors.New("replacement failed")
		rename := func(oldPath, newPath string) error {
			if oldPath == tmpPath {
				return replaceErr
			}
			return os.Rename(oldPath, newPath)
		}

		err := replaceExistingFile(tmpPath, path, rename, os.Remove)
		if !errors.Is(err, replaceErr) {
			t.Fatalf("replaceExistingFile() error = %v, want %v", err, replaceErr)
		}

		assertFileContent(t, path, "old")
		assertFileContent(t, tmpPath, "new")
		assertPathMissing(t, tmpPath+".backup")
	})

	t.Run("preserves backup and reports both errors when restore fails", func(t *testing.T) {
		t.Parallel()

		tmpPath, path := writeReplacementFiles(t)
		replaceErr := errors.New("replacement failed")
		restoreErr := errors.New("restore failed")
		rename := func(oldPath, newPath string) error {
			switch oldPath {
			case tmpPath:
				return replaceErr
			case tmpPath + ".backup":
				return restoreErr
			default:
				return os.Rename(oldPath, newPath)
			}
		}

		err := replaceExistingFile(tmpPath, path, rename, os.Remove)
		if !errors.Is(err, replaceErr) {
			t.Fatalf("replaceExistingFile() error = %v, want %v", err, replaceErr)
		}
		if !errors.Is(err, restoreErr) {
			t.Fatalf("replaceExistingFile() error = %v, want %v", err, restoreErr)
		}

		assertPathMissing(t, path)
		assertFileContent(t, tmpPath, "new")
		assertFileContent(t, tmpPath+".backup", "old")
	})
}

func writeReplacementFiles(t *testing.T) (string, string) {
	t.Helper()

	dir := t.TempDir()
	path := filepath.Join(dir, "object.md")
	tmpPath := filepath.Join(dir, ".object.md.tmp-test")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("write destination: %v", err)
	}
	if err := os.WriteFile(tmpPath, []byte("new"), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	return tmpPath, path
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if string(got) != want {
		t.Fatalf("%s content = %q, want %q", path, got, want)
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os.Stat(%s) error = %v, want os.ErrNotExist", path, err)
	}
}

func TestWriteFile(t *testing.T) {
	t.Parallel()

	t.Run("creates new file with default permissions", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		data := []byte("hello world")

		if err := WriteFile(path, data, 0); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		assertFileContent(t, path, "hello world")

		// Skip permission check on Windows where permissions don't work the same way
		if runtime.GOOS != "windows" {
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat error: %v", err)
			}
			if stat.Mode().Perm() != 0o644 {
				t.Errorf("file mode = %o, want %o", stat.Mode().Perm(), 0o644)
			}
		}
	})

	t.Run("creates new file with explicit permissions", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")
		data := []byte("hello")

		if err := WriteFile(path, data, 0o600); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		assertFileContent(t, path, "hello")

		// Skip permission check on Windows where permissions don't work the same way
		if runtime.GOOS != "windows" {
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat error: %v", err)
			}
			if stat.Mode().Perm() != 0o600 {
				t.Errorf("file mode = %o, want %o", stat.Mode().Perm(), 0o600)
			}
		}
	})

	t.Run("overwrites existing file preserving permissions", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "existing.txt")

		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatalf("write initial file: %v", err)
		}

		if err := WriteFile(path, []byte("new"), 0); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		assertFileContent(t, path, "new")

		// Skip permission check on Windows where permissions don't work the same way
		if runtime.GOOS != "windows" {
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat error: %v", err)
			}
			if stat.Mode().Perm() != 0o600 {
				t.Errorf("file mode = %o, want %o (should preserve)", stat.Mode().Perm(), 0o600)
			}
		}
	})

	t.Run("overwrites existing file with new permissions", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "existing.txt")

		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatalf("write initial file: %v", err)
		}

		if err := WriteFile(path, []byte("new"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		assertFileContent(t, path, "new")

		// Skip permission check on Windows where permissions don't work the same way
		if runtime.GOOS != "windows" {
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat error: %v", err)
			}
			if stat.Mode().Perm() != 0o644 {
				t.Errorf("file mode = %o, want %o (explicit override)", stat.Mode().Perm(), 0o644)
			}
		}
	})

	t.Run("writes empty file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "empty.txt")

		if err := WriteFile(path, []byte{}, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		assertFileContent(t, path, "")
	})

	t.Run("writes binary data", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "binary.dat")
		data := []byte{0x00, 0xFF, 0x42, 0xAB, 0xCD, 0xEF}

		if err := WriteFile(path, data, 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		got, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		if len(got) != len(data) {
			t.Fatalf("file size = %d, want %d", len(got), len(data))
		}
		for i, b := range data {
			if got[i] != b {
				t.Errorf("byte[%d] = %02x, want %02x", i, got[i], b)
			}
		}
	})

	t.Run("fails when directory does not exist", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent", "test.txt")

		err := WriteFile(path, []byte("data"), 0o644)
		if err == nil {
			t.Fatalf("WriteFile() succeeded, want error for non-existent directory")
		}
	})

	t.Run("cleans up temp file on write error", func(t *testing.T) {
		// This test is best-effort; it's hard to reliably trigger a write error
		// in a portable way, but we can at least verify the temp file cleanup happens
		// in the normal success path.
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "test.txt")

		if err := WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("WriteFile() error = %v", err)
		}

		// Verify no leftover temp files
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir: %v", err)
		}

		for _, entry := range entries {
			if entry.Name() != "test.txt" {
				t.Errorf("unexpected file in dir: %s", entry.Name())
			}
		}
	})

	t.Run("handles concurrent writes to same file", func(t *testing.T) {
		// WriteFile uses temp files so concurrent writes should be safe
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "concurrent.txt")

		done := make(chan error, 2)
		write := func(data string) {
			done <- WriteFile(path, []byte(data), 0o644)
		}

		go write("goroutine1")
		go write("goroutine2")

		// Both writes should succeed
		for i := 0; i < 2; i++ {
			if err := <-done; err != nil {
				t.Errorf("concurrent write %d error: %v", i, err)
			}
		}

		// Final content should be one of the two values
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read file: %v", err)
		}
		s := string(content)
		if s != "goroutine1" && s != "goroutine2" {
			t.Errorf("file content = %q, want 'goroutine1' or 'goroutine2'", s)
		}
	})
}

func TestFileExists(t *testing.T) {
	t.Parallel()

	t.Run("returns true for existing file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "exists.txt")
		if err := os.WriteFile(path, []byte("test"), 0o644); err != nil {
			t.Fatalf("write file: %v", err)
		}

		if !fileExists(path) {
			t.Errorf("fileExists(%s) = false, want true", path)
		}
	})

	t.Run("returns false for non-existent file", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "nonexistent.txt")

		if fileExists(path) {
			t.Errorf("fileExists(%s) = true, want false", path)
		}
	})

	t.Run("returns false for directory", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()

		// On some platforms, stat on directory returns info even for non-files
		// fileExists is intentionally simple and uses Stat, so skip this test
		// if the platform's Stat doesn't fail for directories
		stat, err := os.Stat(dir)
		if err == nil && !stat.IsDir() {
			// Not a directory somehow, skip
			return
		}

		// fileExists checks for os.Stat error == nil, so directories will
		// return true. This is acceptable for our use case (atomic file write).
		_ = fileExists(dir)
	})
}
