package vaultwatch

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"
)

func TestWatcherDebouncesRapidWrites(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	note := filepath.Join(vaultPath, "note.md")
	if err := writeFile(note, "one\n"); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var batches []Batch
	applied := make(chan struct{}, 8)
	w, err := Start(context.Background(), Options{
		VaultPath: vaultPath,
		Debounce:  40 * time.Millisecond,
		Apply: func(_ context.Context, batch Batch) error {
			mu.Lock()
			batches = append(batches, batch)
			mu.Unlock()
			applied <- struct{}{}
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Close() }()

	if err := writeFile(note, "two\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(note, "three\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case <-applied:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for debounced apply")
	}
	time.Sleep(80 * time.Millisecond)

	mu.Lock()
	defer mu.Unlock()
	if len(batches) != 1 {
		t.Fatalf("batches = %d, want 1 after debounce: %#v", len(batches), batches)
	}
	if !containsPath(batches[0].Paths, "note.md") {
		t.Fatalf("paths = %#v, want note.md", batches[0].Paths)
	}
}

func TestWatcherIgnoresRavenDir(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	if err := writeFile(filepath.Join(vaultPath, ".raven", "index-dirty.json"), "{}\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(vaultPath, "ok.md"), "ok\n"); err != nil {
		t.Fatal(err)
	}

	applied := make(chan Batch, 8)
	w, err := Start(context.Background(), Options{
		VaultPath: vaultPath,
		Debounce:  20 * time.Millisecond,
		Apply: func(_ context.Context, batch Batch) error {
			applied <- batch
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Close() }()

	if err := writeFile(filepath.Join(vaultPath, ".raven", "noise.txt"), "x\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(vaultPath, "ok.md"), "changed\n"); err != nil {
		t.Fatal(err)
	}

	select {
	case batch := <-applied:
		if containsPath(batch.Paths, ".raven/noise.txt") {
			t.Fatalf("watcher applied .raven path: %#v", batch.Paths)
		}
		if !containsPath(batch.Paths, "ok.md") {
			t.Fatalf("paths = %#v, want ok.md", batch.Paths)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for apply")
	}
}

func TestWatcherDoesNotFollowDirectorySymlink(t *testing.T) {
	t.Parallel()
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests are not reliable on windows")
	}

	root := t.TempDir()
	vaultPath := filepath.Join(root, "vault")
	outside := filepath.Join(root, "outside")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}

	applied := make(chan Batch, 8)
	w, err := Start(context.Background(), Options{
		VaultPath: vaultPath,
		Debounce:  20 * time.Millisecond,
		Apply: func(_ context.Context, batch Batch) error {
			applied <- batch
			return nil
		},
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Close() }()

	link := filepath.Join(vaultPath, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}
	if err := writeFile(filepath.Join(outside, "secret.md"), "outside\n"); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(filepath.Join(vaultPath, "ok.md"), "inside\n"); err != nil {
		t.Fatal(err)
	}

	deadline := time.After(time.Second)
	for {
		select {
		case batch := <-applied:
			if containsPath(batch.Paths, "secret.md") || containsPath(batch.Paths, "link/secret.md") {
				t.Fatalf("watcher followed directory symlink: %#v", batch.Paths)
			}
			if containsPath(batch.Paths, "ok.md") {
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for in-vault apply")
		}
	}
}

func TestMergeBatchesPromotesOverflow(t *testing.T) {
	t.Parallel()
	got := mergeBatches(Batch{Paths: []string{"a.md"}}, Batch{Overflow: true})
	if !got.Overflow {
		t.Fatalf("merged = %#v, want overflow", got)
	}
}

func writeFile(path, content string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func containsPath(paths []string, want string) bool {
	for _, got := range paths {
		if got == want {
			return true
		}
	}
	return false
}
