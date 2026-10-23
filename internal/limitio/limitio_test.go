package limitio

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadAllAcceptsExactLimit(t *testing.T) {
	t.Parallel()
	data, err := ReadAll(strings.NewReader("abcd"), 4)
	if err != nil {
		t.Fatalf("ReadAll() = %v", err)
	}
	if string(data) != "abcd" {
		t.Fatalf("ReadAll() = %q, want abcd", data)
	}
}

func TestReadAllRejectsOverLimit(t *testing.T) {
	t.Parallel()
	_, err := ReadAll(strings.NewReader("abcde"), 4)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadAll() err = %v, want ErrTooLarge", err)
	}
}

func TestReadFileRejectsOverLimit(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "big.txt")
	if err := os.WriteFile(path, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadFile(path, 4)
	if !errors.Is(err, ErrTooLarge) {
		t.Fatalf("ReadFile() err = %v, want ErrTooLarge", err)
	}
}

func TestReadFileMissing(t *testing.T) {
	t.Parallel()
	_, err := ReadFile(filepath.Join(t.TempDir(), "missing"), 16)
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("ReadFile() err = %v, want not exist", err)
	}
}

func TestReadAllEmpty(t *testing.T) {
	t.Parallel()
	data, err := ReadAll(bytes.NewReader(nil), 8)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("len = %d, want 0", len(data))
	}
}
