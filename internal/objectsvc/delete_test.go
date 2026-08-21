package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestDeleteFileTrashMovesFile(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  ".trash",
	})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if result.Behavior != "trash" {
		t.Fatalf("expected trash behavior, got %q", result.Behavior)
	}
	if result.TrashPath == "" {
		t.Fatal("expected trash path")
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected source to be moved, stat err=%v", err)
	}
	if _, err := os.Stat(result.TrashPath); err != nil {
		t.Fatalf("expected trashed file: %v", err)
	}
}

func TestDeleteFileTrashCollisionAddsTimestamp(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	trashPath := filepath.Join(vaultPath, ".trash/people/freya.md")

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		t.Fatalf("mkdir trash: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("new"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}
	if err := os.WriteFile(trashPath, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed existing trash file: %v", err)
	}

	now := time.Date(2026, 3, 10, 11, 22, 33, 0, time.UTC)
	result, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  ".trash",
		Now: func() time.Time {
			return now
		},
	})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	tag := trashCollisionTag("people/freya.md")
	expected := filepath.Join(vaultPath, ".trash/people/freya.raven-trash-"+tag+"-2026-03-10-112233-1.md")
	if result.TrashPath != expected {
		t.Fatalf("expected timestamped trash path %q, got %q", expected, result.TrashPath)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected timestamped trashed file: %v", err)
	}
}

func TestDeleteFileTrashCollisionAllocatesSequenceWithoutOverwrite(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	trashPath := filepath.Join(vaultPath, ".trash/people/freya.md")
	tag := trashCollisionTag("people/freya.md")
	firstVersion := filepath.Join(vaultPath, ".trash/people/freya.raven-trash-"+tag+"-2026-03-10-112233-1.md")

	for path, content := range map[string]string{
		filePath:     "newest",
		trashPath:    "oldest",
		firstVersion: "older",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", path, err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", path, err)
		}
	}

	now := time.Date(2026, 3, 10, 11, 22, 33, 0, time.UTC)
	result, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  ".trash",
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}

	expected := filepath.Join(vaultPath, ".trash/people/freya.raven-trash-"+tag+"-2026-03-10-112233-2.md")
	if result.TrashPath != expected {
		t.Fatalf("TrashPath = %q, want %q", result.TrashPath, expected)
	}
	for path, want := range map[string]string{
		trashPath:    "oldest",
		firstVersion: "older",
		expected:     "newest",
	} {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		if string(content) != want {
			t.Fatalf("%s content = %q, want %q", path, content, want)
		}
	}
}

func TestDeleteFilePermanentRemovesFile(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "permanent",
	})
	if err != nil {
		t.Fatalf("DeleteFile: %v", err)
	}
	if result.Behavior != "permanent" {
		t.Fatalf("expected permanent behavior, got %q", result.Behavior)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("expected source to be removed, stat err=%v", err)
	}
}

func TestDeleteFileInvalidBehavior(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	_, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "invalid",
	})
	if err == nil {
		t.Fatal("expected error")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrInvalidInput {
		t.Fatalf("expected ErrorInvalidInput, got %s", svcErr.Code)
	}
}

func TestDeleteFileRejectsUnsafeTrashDirectories(t *testing.T) {
	t.Parallel()

	tests := []string{
		"../outside",
		"/tmp/outside",
		`C:\outside`,
		".raven/trash",
		".git/trash",
		".RAVEN/trash",
		".GIT/trash",
	}
	for _, trashDir := range tests {
		t.Run(trashDir, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			filePath := filepath.Join(vaultPath, "people/freya.md")
			if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
				t.Fatalf("seed file: %v", err)
			}

			_, err := DeleteFile(DeleteFileRequest{
				VaultPath: vaultPath,
				FilePath:  filePath,
				Behavior:  "trash",
				TrashDir:  trashDir,
			})
			if err == nil {
				t.Fatal("DeleteFile() succeeded with unsafe trash directory")
			}
			var serviceErr *svcerr.Error
			if !errors.As(err, &serviceErr) || serviceErr.Code != codes.ErrConfigInvalid {
				t.Fatalf("error = %v, want %s", err, codes.ErrConfigInvalid)
			}
			if _, statErr := os.Stat(filePath); statErr != nil {
				t.Fatalf("source changed after rejected delete: %v", statErr)
			}
		})
	}
}

func TestDeleteFileRejectsSymlinkedTrashDirectory(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	realTrash := filepath.Join(vaultPath, "real-trash")
	if err := os.MkdirAll(realTrash, 0o755); err != nil {
		t.Fatalf("mkdir real trash: %v", err)
	}
	if err := os.Symlink(realTrash, filepath.Join(vaultPath, "linked-trash")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	_, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  "linked-trash",
	})
	if err == nil {
		t.Fatal("DeleteFile() succeeded with symlinked trash directory")
	}
	var serviceErr *svcerr.Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != codes.ErrConfigInvalid {
		t.Fatalf("error = %v, want %s", err, codes.ErrConfigInvalid)
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatalf("source changed after rejected delete: %v", statErr)
	}
}

func TestDeleteFileNormalizesRelativeBackslashTrashDirectory(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	result, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  `archive\trash`,
	})
	if err != nil {
		t.Fatalf("DeleteFile() error = %v", err)
	}
	expected := filepath.Join(vaultPath, "archive/trash/people/freya.md")
	if result.TrashPath != expected {
		t.Fatalf("TrashPath = %q, want %q", result.TrashPath, expected)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("normalized trash entry missing: %v", err)
	}
	vaultCfg := &config.VaultConfig{Deletion: &config.DeletionConfig{TrashDir: `archive\trash`}}
	listed, err := ListTrash(ListTrashRequest{VaultPath: vaultPath, VaultConfig: vaultCfg})
	if err != nil {
		t.Fatalf("ListTrash() error = %v", err)
	}
	if len(listed.Entries) != 1 || listed.Entries[0].TrashPath != "archive/trash/people/freya.md" {
		t.Fatalf("entries = %#v, want normalized trash path", listed.Entries)
	}
	if _, err := RestoreByReference(RestoreByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Reference:   "people/freya",
	}); err != nil {
		t.Fatalf("RestoreByReference() error = %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("restored file missing: %v", err)
	}
}

func TestDeleteFileRejectsSymlinkBelowTrashRoot(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	outsidePath := t.TempDir()
	trashRoot := filepath.Join(vaultPath, ".trash")
	if err := os.MkdirAll(trashRoot, 0o755); err != nil {
		t.Fatalf("mkdir trash root: %v", err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(trashRoot, "people")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir source: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("hello"), 0o644); err != nil {
		t.Fatalf("seed source: %v", err)
	}

	_, err := DeleteFile(DeleteFileRequest{
		VaultPath: vaultPath,
		FilePath:  filePath,
		Behavior:  "trash",
		TrashDir:  ".trash",
	})
	if err == nil {
		t.Fatal("DeleteFile() succeeded through symlinked trash parent")
	}
	var serviceErr *svcerr.Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != codes.ErrConfigInvalid {
		t.Fatalf("error = %v, want %s", err, codes.ErrConfigInvalid)
	}
	if _, statErr := os.Stat(filePath); statErr != nil {
		t.Fatalf("source changed after rejected delete: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(outsidePath, "freya.md")); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("file escaped through trash symlink, stat error = %v", statErr)
	}
}
