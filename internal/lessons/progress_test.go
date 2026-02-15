package lessons

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProgressMissingReturnsEmpty(t *testing.T) {
	vaultPath := t.TempDir()

	progress, err := LoadProgress(vaultPath)
	if err != nil {
		t.Fatalf("LoadProgress() error = %v", err)
	}

	if progress.IsCompleted("objects") {
		t.Fatalf("expected empty progress to report incomplete lessons")
	}
}

func TestSaveAndLoadProgress(t *testing.T) {
	vaultPath := t.TempDir()

	progress := NewProgress()
	if already := progress.MarkCompleted("objects", "2026-02-15"); already {
		t.Fatalf("expected first completion to be new")
	}
	if already := progress.MarkCompleted("objects", "2026-02-16"); !already {
		t.Fatalf("expected second completion to be idempotent")
	}
	progress.MarkCompleted("refs", "2026-02-16")

	if err := SaveProgress(vaultPath, progress); err != nil {
		t.Fatalf("SaveProgress() error = %v", err)
	}

	progressPath := filepath.Join(vaultPath, ".raven", "learn", "progress.yaml")
	if _, err := os.Stat(progressPath); err != nil {
		t.Fatalf("expected progress file to exist: %v", err)
	}

	loaded, err := LoadProgress(vaultPath)
	if err != nil {
		t.Fatalf("LoadProgress() after save error = %v", err)
	}

	if !loaded.IsCompleted("objects") {
		t.Fatalf("expected objects to be completed")
	}
	if date, ok := loaded.CompletedDate("objects"); !ok || date != "2026-02-15" {
		t.Fatalf("expected objects completion date 2026-02-15, got %q (ok=%v)", date, ok)
	}
	if !loaded.IsCompleted("refs") {
		t.Fatalf("expected refs to be completed")
	}
}
