package atomicfile

import (
	"errors"
	"os"
	"path/filepath"
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
