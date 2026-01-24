package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeDirRoot(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{"", ""},
		{"/", ""},
		{"objects", "objects/"},
		{"objects/", "objects/"},
		{"/objects/", "objects/"},
		{"objects//", "objects/"},
	}
	for _, tc := range tests {
		if got := NormalizeDirRoot(tc.in); got != tc.want {
			t.Fatalf("NormalizeDirRoot(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestFilePathToObjectID(t *testing.T) {
	tests := []struct {
		filePath    string
		objectsRoot string
		pagesRoot   string
		want        string
	}{
		{"people/freya.md", "", "", "people/freya"},
		{"./people/freya.md", "", "", "people/freya"},
		{"/people/freya.md", "", "", "people/freya"},
		{"objects/people/freya.md", "objects/", "pages/", "people/freya"},
		{"pages/my-note.md", "objects/", "pages/", "my-note"},
		{"daily/2025-01-01.md", "objects/", "pages/", "daily/2025-01-01"},
		// If a root isn't configured, it should not be stripped.
		{"objects/people/freya.md", "", "pages/", "objects/people/freya"},
	}
	for _, tc := range tests {
		if got := FilePathToObjectID(tc.filePath, tc.objectsRoot, tc.pagesRoot); got != tc.want {
			t.Fatalf("FilePathToObjectID(%q, %q, %q) = %q, want %q", tc.filePath, tc.objectsRoot, tc.pagesRoot, got, tc.want)
		}
	}
}

func TestObjectIDToFilePath(t *testing.T) {
	tests := []struct {
		id          string
		typeName    string
		objectsRoot string
		pagesRoot   string
		want        string
	}{
		{"people/freya", "person", "objects/", "pages/", "objects/people/freya.md"},
		{"my-note", "page", "objects/", "pages/", "pages/my-note.md"},
		{"random-note", "", "objects/", "pages/", "pages/random-note.md"},
		// pages root missing falls back to objects root for pages.
		{"my-note", "page", "objects/", "", "objects/my-note.md"},
		// Already-rooted input should not be double-prefixed.
		{"objects/people/freya", "person", "objects/", "pages/", "objects/people/freya.md"},
	}
	for _, tc := range tests {
		if got := ObjectIDToFilePath(tc.id, tc.typeName, tc.objectsRoot, tc.pagesRoot); got != tc.want {
			t.Fatalf("ObjectIDToFilePath(%q, %q, %q, %q) = %q, want %q", tc.id, tc.typeName, tc.objectsRoot, tc.pagesRoot, got, tc.want)
		}
	}
}

func TestCandidateFilePaths(t *testing.T) {
	got := CandidateFilePaths("people/freya", "objects/", "pages/")
	// Always includes literal, objects-rooted, pages-rooted.
	want := map[string]struct{}{
		"people/freya.md":         {},
		"objects/people/freya.md": {},
		"pages/people/freya.md":   {},
	}
	if len(got) != 3 {
		t.Fatalf("got %d candidates, want 3: %#v", len(got), got)
	}
	for _, p := range got {
		if _, ok := want[p]; !ok {
			t.Fatalf("unexpected candidate %q (got=%#v)", p, got)
		}
	}
}

func TestValidateWithinVault_AllowsInside(t *testing.T) {
	vaultDir := t.TempDir()
	target := filepath.Join(vaultDir, "notes", "new.md")
	if err := ValidateWithinVault(vaultDir, target); err != nil {
		t.Fatalf("ValidateWithinVault() = %v, want nil", err)
	}
}

func TestValidateWithinVault_SymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink tests are not reliable on windows")
	}

	rootDir := t.TempDir()
	vaultDir := filepath.Join(rootDir, "vault")
	outsideDir := filepath.Join(rootDir, "outside")

	if err := os.MkdirAll(vaultDir, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.MkdirAll(outsideDir, 0o755); err != nil {
		t.Fatalf("mkdir outside: %v", err)
	}

	linkPath := filepath.Join(vaultDir, "link")
	if err := os.Symlink(outsideDir, linkPath); err != nil {
		t.Skipf("symlink not supported: %v", err)
	}

	target := filepath.Join(linkPath, "new.md")
	err := ValidateWithinVault(vaultDir, target)
	if !errors.Is(err, ErrPathOutsideVault) {
		t.Fatalf("ValidateWithinVault() = %v, want ErrPathOutsideVault", err)
	}
}
