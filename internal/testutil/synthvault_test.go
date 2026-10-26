package testutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteSynthVault(t *testing.T) {
	t.Parallel()

	opts := SynthVaultOptions{People: 3, Projects: 2, Notes: 1, Daily: 1}
	dir := t.TempDir()
	if err := WriteSynthVault(dir, opts); err != nil {
		t.Fatalf("WriteSynthVault: %v", err)
	}
	count, err := CountMarkdownFiles(dir)
	if err != nil {
		t.Fatalf("CountMarkdownFiles: %v", err)
	}
	if count != opts.FileCount() {
		t.Fatalf("markdown files = %d, want %d", count, opts.FileCount())
	}
	for _, rel := range []string{"schema.yaml", "raven.yaml", "people/p0000.md", "projects/proj-0000.md"} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("missing %s: %v", rel, err)
		}
	}
}
