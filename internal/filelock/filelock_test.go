//go:build !windows

package filelock

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestLockExclusive(t *testing.T) {
	t.Parallel()

	t.Run("acquires exclusive lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		defer f.Close()

		if err := LockExclusive(f); err != nil {
			t.Fatalf("LockExclusive() error = %v", err)
		}

		if err := Unlock(f); err != nil {
			t.Fatalf("Unlock() error = %v", err)
		}
	})

	t.Run("blocks when another exclusive lock held", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := LockExclusive(f1); err != nil {
			t.Fatalf("LockExclusive(f1) error = %v", err)
		}
		defer Unlock(f1)

		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		// TryLockExclusive should fail immediately
		err = TryLockExclusive(f2)
		if err == nil {
			t.Fatalf("TryLockExclusive(f2) succeeded, want error (lock already held)")
		}
		if !IsWouldBlock(err) {
			t.Errorf("TryLockExclusive error = %v, want would-block error", err)
		}
	})
}

func TestTryLockExclusive(t *testing.T) {
	t.Parallel()

	t.Run("acquires exclusive lock immediately", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		defer f.Close()

		if err := TryLockExclusive(f); err != nil {
			t.Fatalf("TryLockExclusive() error = %v", err)
		}

		if err := Unlock(f); err != nil {
			t.Fatalf("Unlock() error = %v", err)
		}
	})

	t.Run("fails immediately when lock held", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockExclusive(f1); err != nil {
			t.Fatalf("TryLockExclusive(f1) error = %v", err)
		}
		defer Unlock(f1)

		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		start := time.Now()
		err = TryLockExclusive(f2)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatalf("TryLockExclusive(f2) succeeded, want error")
		}
		if !IsWouldBlock(err) {
			t.Errorf("TryLockExclusive error = %v, want would-block error", err)
		}
		if elapsed > 100*time.Millisecond {
			t.Errorf("TryLockExclusive took %v, want immediate failure", elapsed)
		}
	})
}

func TestTryLockShared(t *testing.T) {
	t.Parallel()

	t.Run("acquires shared lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file: %v", err)
		}
		defer f.Close()

		if err := TryLockShared(f); err != nil {
			t.Fatalf("TryLockShared() error = %v", err)
		}

		if err := Unlock(f); err != nil {
			t.Fatalf("Unlock() error = %v", err)
		}
	})

	t.Run("multiple shared locks allowed", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockShared(f1); err != nil {
			t.Fatalf("TryLockShared(f1) error = %v", err)
		}
		defer Unlock(f1)

		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		if err := TryLockShared(f2); err != nil {
			t.Fatalf("TryLockShared(f2) error = %v, want success (multiple shared locks allowed)", err)
		}
		defer Unlock(f2)
	})

	t.Run("shared lock blocked by exclusive lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockExclusive(f1); err != nil {
			t.Fatalf("TryLockExclusive(f1) error = %v", err)
		}
		defer Unlock(f1)

		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		err = TryLockShared(f2)
		if err == nil {
			t.Fatalf("TryLockShared(f2) succeeded, want error (exclusive lock held)")
		}
		if !IsWouldBlock(err) {
			t.Errorf("TryLockShared error = %v, want would-block error", err)
		}
	})

	t.Run("exclusive lock blocked by shared lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockShared(f1); err != nil {
			t.Fatalf("TryLockShared(f1) error = %v", err)
		}
		defer Unlock(f1)

		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		err = TryLockExclusive(f2)
		if err == nil {
			t.Fatalf("TryLockExclusive(f2) succeeded, want error (shared lock held)")
		}
		if !IsWouldBlock(err) {
			t.Errorf("TryLockExclusive error = %v, want would-block error", err)
		}
	})
}

func TestUnlock(t *testing.T) {
	t.Parallel()

	t.Run("unlocks exclusive lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockExclusive(f1); err != nil {
			t.Fatalf("TryLockExclusive(f1) error = %v", err)
		}

		if err := Unlock(f1); err != nil {
			t.Fatalf("Unlock(f1) error = %v", err)
		}

		// Should be able to lock again after unlock
		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		if err := TryLockExclusive(f2); err != nil {
			t.Fatalf("TryLockExclusive(f2) error = %v, want success (lock was unlocked)", err)
		}
		defer Unlock(f2)
	})

	t.Run("unlocks shared lock", func(t *testing.T) {
		t.Parallel()

		dir := t.TempDir()
		path := filepath.Join(dir, "lock.txt")
		f1, err := os.Create(path)
		if err != nil {
			t.Fatalf("create file 1: %v", err)
		}
		defer f1.Close()

		if err := TryLockShared(f1); err != nil {
			t.Fatalf("TryLockShared(f1) error = %v", err)
		}

		if err := Unlock(f1); err != nil {
			t.Fatalf("Unlock(f1) error = %v", err)
		}

		// Should be able to get exclusive lock after shared is unlocked
		f2, err := os.OpenFile(path, os.O_RDWR, 0644)
		if err != nil {
			t.Fatalf("open file 2: %v", err)
		}
		defer f2.Close()

		if err := TryLockExclusive(f2); err != nil {
			t.Fatalf("TryLockExclusive(f2) error = %v, want success (shared lock was unlocked)", err)
		}
		defer Unlock(f2)
	})
}

func TestIsWouldBlock(t *testing.T) {
	t.Parallel()

	t.Run("detects EWOULDBLOCK", func(t *testing.T) {
		if !IsWouldBlock(syscall.EWOULDBLOCK) {
			t.Errorf("IsWouldBlock(EWOULDBLOCK) = false, want true")
		}
	})

	t.Run("detects EAGAIN", func(t *testing.T) {
		if !IsWouldBlock(syscall.EAGAIN) {
			t.Errorf("IsWouldBlock(EAGAIN) = false, want true")
		}
	})

	t.Run("rejects other errors", func(t *testing.T) {
		if IsWouldBlock(syscall.EINVAL) {
			t.Errorf("IsWouldBlock(EINVAL) = true, want false")
		}
		if IsWouldBlock(syscall.ENOENT) {
			t.Errorf("IsWouldBlock(ENOENT) = true, want false")
		}
	})

	t.Run("rejects nil", func(t *testing.T) {
		if IsWouldBlock(nil) {
			t.Errorf("IsWouldBlock(nil) = true, want false")
		}
	})
}

func TestLockReleasedOnClose(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "lock.txt")
	f1, err := os.Create(path)
	if err != nil {
		t.Fatalf("create file 1: %v", err)
	}

	if err := TryLockExclusive(f1); err != nil {
		t.Fatalf("TryLockExclusive(f1) error = %v", err)
	}

	// Close without explicit unlock
	if err := f1.Close(); err != nil {
		t.Fatalf("Close(f1) error = %v", err)
	}

	// Lock should be automatically released
	f2, err := os.OpenFile(path, os.O_RDWR, 0644)
	if err != nil {
		t.Fatalf("open file 2: %v", err)
	}
	defer f2.Close()

	if err := TryLockExclusive(f2); err != nil {
		t.Fatalf("TryLockExclusive(f2) error = %v, want success (lock released on close)", err)
	}
	defer Unlock(f2)
}
