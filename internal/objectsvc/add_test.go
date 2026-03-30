package objectsvc

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/filelock"
)

func TestAppendToFileWaitsForExclusiveLock(t *testing.T) {
	t.Parallel()

	destPath := filepath.Join(t.TempDir(), "target.md")
	if err := os.WriteFile(destPath, []byte("existing"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	lockFile, err := os.OpenFile(destPath, os.O_RDWR, 0o644)
	if err != nil {
		t.Fatalf("open lock file: %v", err)
	}
	defer lockFile.Close()

	if err := filelock.LockExclusive(lockFile); err != nil {
		t.Fatalf("lock file: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- AppendToFile("", destPath, "appended", nil, nil, false, "", nil)
	}()

	select {
	case err := <-done:
		t.Fatalf("append completed before lock release: %v", err)
	case <-time.After(100 * time.Millisecond):
	}

	if err := filelock.Unlock(lockFile); err != nil {
		t.Fatalf("unlock file: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("append failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("append did not complete after lock release")
	}

	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	if got, want := string(content), "existing\nappended\n"; got != want {
		t.Fatalf("content = %q, want %q", got, want)
	}
}
