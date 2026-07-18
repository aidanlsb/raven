package paths

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestNormalizeDirRoot(t *testing.T) {
	t.Parallel()
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

func TestNormalizeVaultRelPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{" notes//daily.md ", "notes/daily.md", true},
		{"./notes/onboard.md", "notes/onboard.md", true},
		{"/templates/meeting.md", "templates/meeting.md", true},
		{"", ".", false},
		{".", ".", false},
		{"..", "..", false},
		{"../outside", "../outside", false},
	}

	for _, tc := range tests {
		if got := NormalizeVaultRelPath(tc.in); got != tc.want {
			t.Fatalf("NormalizeVaultRelPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
		if got := IsValidVaultRelPath(tc.in); got != tc.ok {
			t.Fatalf("IsValidVaultRelPath(%q) = %v, want %v", tc.in, got, tc.ok)
		}
	}
}

func TestVaultRootFilePaths(t *testing.T) {
	t.Parallel()
	vaultPath := "/tmp/test-vault"
	if got, want := SchemaPath(vaultPath), filepath.Join(vaultPath, "schema.yaml"); got != want {
		t.Fatalf("SchemaPath() = %q, want %q", got, want)
	}
	if got, want := AgentInstructionsPath(vaultPath), filepath.Join(vaultPath, "AGENTS.md"); got != want {
		t.Fatalf("AgentInstructionsPath() = %q, want %q", got, want)
	}
}

func TestFilePathToObjectID(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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

func TestReferenceToFilePath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		ref         string
		objectsRoot string
		pagesRoot   string
		want        string
	}{
		// With both roots configured.
		{"slash means objects root", "person/freya", "objects/", "pages/", "objects/person/freya.md"},
		{"bare name means pages root", "my-note", "objects/", "pages/", "pages/my-note.md"},
		{"already objects-rooted kept as-is", "objects/person/freya", "objects/", "pages/", "objects/person/freya.md"},
		{"already pages-rooted kept as-is", "pages/my-note", "objects/", "pages/", "pages/my-note.md"},
		{"trailing .md stripped then re-added", "objects/person/freya.md", "objects/", "pages/", "objects/person/freya.md"},

		// Trailing-slash / normalization edge cases on the roots themselves.
		{"roots without trailing slash still normalized", "person/freya", "objects", "pages", "objects/person/freya.md"},
		{"leading slash ref normalized", "/person/freya", "objects/", "pages/", "objects/person/freya.md"},
		{"dot-slash ref normalized", "./my-note", "objects/", "pages/", "pages/my-note.md"},

		// Pages root missing falls back to objects root for bare names.
		{"bare name falls back to objects root", "my-note", "objects/", "", "objects/my-note.md"},

		// No roots configured: literal interpretation only.
		{"no roots, slash", "person/freya", "", "", "person/freya.md"},
		{"no roots, bare", "my-note", "", "", "my-note.md"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReferenceToFilePath(tc.ref, tc.objectsRoot, tc.pagesRoot); got != tc.want {
				t.Fatalf("ReferenceToFilePath(%q, %q, %q) = %q, want %q", tc.ref, tc.objectsRoot, tc.pagesRoot, got, tc.want)
			}
		})
	}
}

func TestCandidateFilePaths(t *testing.T) {
	t.Parallel()
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

func TestRelFromVault(t *testing.T) {
	t.Parallel()
	vault := filepath.Join("/tmp", "vault")
	tests := []struct {
		target string
		want   string
	}{
		{filepath.Join(vault, "objects", "people", "freya.md"), "objects/people/freya.md"},
		{filepath.Join(vault, "note.md"), "note.md"},
		{vault, "."},
	}
	for _, tc := range tests {
		got, err := RelFromVault(vault, tc.target)
		if err != nil {
			t.Fatalf("RelFromVault(%q, %q) error: %v", vault, tc.target, err)
		}
		if got != tc.want {
			t.Fatalf("RelFromVault(%q, %q) = %q, want %q", vault, tc.target, got, tc.want)
		}
	}
}

func TestJoinDirRoot(t *testing.T) {
	t.Parallel()
	tests := []struct {
		root string
		rel  string
		want string
	}{
		{"objects/", "people/freya", "objects/people/freya"},
		{"objects", "people/freya", "objects/people/freya"},
		{"/objects/", "people/freya", "objects/people/freya"},
		{"", "people/freya", "people/freya"},
		{"objects/", "notes/../people/freya", "objects/people/freya"},
	}
	for _, tc := range tests {
		if got := JoinDirRoot(tc.root, tc.rel); got != tc.want {
			t.Fatalf("JoinDirRoot(%q, %q) = %q, want %q", tc.root, tc.rel, got, tc.want)
		}
	}
}

func TestSanitizeDefaultPath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want string
		ok   bool
	}{
		{"notes/", "notes", true},
		{"notes", "notes", true},
		{"projects/active", "projects/active", true},
		{"a/../b", "b", true},
		{"", "", false},
		{".", "", false},
		{"..", "", false},
		{"../escape", "", false},
		{"notes/../../escape", "", false},
		{"/abs", "", false},
	}
	for _, tc := range tests {
		got, ok := SanitizeDefaultPath(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("SanitizeDefaultPath(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestIsCleanRelSubpath(t *testing.T) {
	t.Parallel()
	tests := []struct {
		in   string
		want bool
	}{
		{"getting-started/intro.md", true},
		{"intro.md", true},
		{"", false},
		{".", false},
		{"..", false},
		{"../outside.md", false},
		{"/abs.md", false},
	}
	for _, tc := range tests {
		if got := IsCleanRelSubpath(tc.in); got != tc.want {
			t.Fatalf("IsCleanRelSubpath(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestValidateWithinVault_AllowsInside(t *testing.T) {
	t.Parallel()
	vaultDir := t.TempDir()
	target := filepath.Join(vaultDir, "notes", "new.md")
	if err := ValidateWithinVault(vaultDir, target); err != nil {
		t.Fatalf("ValidateWithinVault() = %v, want nil", err)
	}
}

func TestValidateWithinVault_SymlinkEscape(t *testing.T) {
	t.Parallel()
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
