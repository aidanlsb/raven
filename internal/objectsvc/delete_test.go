package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
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

	expected := filepath.Join(vaultPath, ".trash/people/freya-2026-03-10-112233.md")
	if result.TrashPath != expected {
		t.Fatalf("expected timestamped trash path %q, got %q", expected, result.TrashPath)
	}
	if _, err := os.Stat(expected); err != nil {
		t.Fatalf("expected timestamped trashed file: %v", err)
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

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorInvalidInput {
		t.Fatalf("expected ErrorInvalidInput, got %s", svcErr.Code)
	}
}
