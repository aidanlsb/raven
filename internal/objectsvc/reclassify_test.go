package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestReclassifyMoveWritesUpdatedContentAtDestination(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	sourcePath := filepath.Join(vaultPath, "notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := Reclassify(ReclassifyRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectID:    "notes/my-note",
		FilePath:    sourcePath,
		NewTypeName: "book",
		Force:       true,
	})
	if err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}
	if !result.Moved {
		t.Fatalf("expected moved result, got %#v", result)
	}

	destPath := filepath.Join(vaultPath, "books/my-note.md")
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if !strings.Contains(string(content), "type: book") {
		t.Fatalf("expected reclassified type in moved file, got:\n%s", string(content))
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("expected source file removed, err=%v", err)
	}
}

func TestReclassifyMovePlansCanonicalDestinationWithDirectories(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	// Directories config: typed items live under "type/", pages under "page/".
	vaultCfg := &config.VaultConfig{
		DailyDirectory: "daily",
		Directories: &config.DirectoriesConfig{
			Object: "type/",
			Page:   "page/",
		},
	}

	sourcePath := filepath.Join(vaultPath, "type/notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := Reclassify(ReclassifyRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Schema:      sch,
		ObjectID:    "notes/my-note",
		FilePath:    sourcePath,
		NewTypeName: "book",
		Force:       true,
	})
	if err != nil {
		t.Fatalf("Reclassify() error = %v", err)
	}
	if !result.Moved {
		t.Fatalf("expected moved result, got %#v", result)
	}
	// Canonical destination applies the object root and the new type's
	// default_path: type/ + books/ + my-note.md.
	if result.NewPath != "type/books/my-note.md" {
		t.Fatalf("unexpected NewPath %q, want type/books/my-note.md", result.NewPath)
	}

	destPath := filepath.Join(vaultPath, "type/books/my-note.md")
	content, err := os.ReadFile(destPath)
	if err != nil {
		t.Fatalf("read destination: %v", err)
	}
	if !strings.Contains(string(content), "type: book") {
		t.Fatalf("expected reclassified type in moved file, got:\n%s", string(content))
	}
	if _, err := os.Stat(sourcePath); !os.IsNotExist(err) {
		t.Fatalf("expected source file removed, err=%v", err)
	}
	if result.ObjectID != "books/my-note" {
		t.Fatalf("expected destination object ID 'books/my-note', got %q", result.ObjectID)
	}
}

// TestReclassifyGuardsForbiddenDestinations exercises the reclassify path
// that computes destinations from the new type's default_path. Without the
// MoveFile guard, these would move files into templates/, protected
// prefixes, or excluded directories.
func TestReclassifyGuardsForbiddenDestinations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		vaultCfg      *config.VaultConfig
		defaultPath   string
		wantErrSubstr string
	}{
		{
			name:          "default_path targets templates directory",
			vaultCfg:      &config.VaultConfig{},
			defaultPath:   "templates/",
			wantErrSubstr: "template files",
		},
		{
			name:          "default_path targets protected prefix",
			vaultCfg:      &config.VaultConfig{ProtectedPrefixes: []string{"private/"}},
			defaultPath:   "private/",
			wantErrSubstr: "protected or system-managed",
		},
		{
			name:          "default_path targets excluded directory",
			vaultCfg:      &config.VaultConfig{Exclude: []string{"archive/"}},
			defaultPath:   "archive/",
			wantErrSubstr: "excluded paths",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vaultPath := t.TempDir()
			writeTestSchema(t, vaultPath, fmt.Sprintf(`
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: %s
    fields:
      title:
        type: string
traits: {}
`, tt.defaultPath))
			sch := loadTestSchema(t, vaultPath)

			sourcePath := filepath.Join(vaultPath, "notes/my-note.md")
			if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
				t.Fatalf("mkdir: %v", err)
			}
			if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
				t.Fatalf("seed file: %v", err)
			}

			_, err := Reclassify(ReclassifyRequest{
				VaultPath:   vaultPath,
				VaultConfig: tt.vaultCfg,
				Schema:      sch,
				ObjectID:    "notes/my-note",
				FilePath:    sourcePath,
				NewTypeName: "book",
				Force:       true,
			})
			if err == nil {
				t.Fatalf("Reclassify() error = nil, want validation failure")
			}

			var svcErr *svcerr.Error
			if !errors.As(err, &svcErr) {
				t.Fatalf("expected *Error, got %T: %v", err, err)
			}
			if svcErr.Code != codes.ErrValidationFailed {
				t.Errorf("error code = %s, want %s", svcErr.Code, codes.ErrValidationFailed)
			}
			if !strings.Contains(svcErr.Message, tt.wantErrSubstr) {
				t.Errorf("error message = %q, want to contain %q", svcErr.Message, tt.wantErrSubstr)
			}

			// Source must remain intact; nothing should be written to the
			// forbidden destination directory.
			content, readErr := os.ReadFile(sourcePath)
			if readErr != nil {
				t.Fatalf("read source after failure: %v", readErr)
			}
			if !strings.Contains(string(content), "type: note") {
				t.Fatalf("source file rewritten despite guard failure: %s", content)
			}
			forbiddenDir := filepath.Join(vaultPath, strings.TrimSuffix(tt.defaultPath, "/"))
			if entries, err := os.ReadDir(forbiddenDir); err == nil {
				for _, entry := range entries {
					if entry.Name() == "my-note.md" {
						t.Errorf("expected no file in forbidden destination %s, found my-note.md", forbiddenDir)
					}
				}
			}
		})
	}
}

func TestReclassifyMoveFailureLeavesSourceUntouched(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields: {}
  book:
    default_path: books/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	sourcePath := filepath.Join(vaultPath, "notes/my-note.md")
	if err := os.MkdirAll(filepath.Dir(sourcePath), 0o755); err != nil {
		t.Fatalf("mkdir source dir: %v", err)
	}
	if err := os.WriteFile(sourcePath, []byte("---\ntype: note\ntitle: My Note\n---\n\nContent.\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	failPath := filepath.Join(vaultPath, "books/my-note.md")
	restoreWriter := swapMoveFileWriterForTest(func(path string, data []byte, perm os.FileMode) error {
		if path == failPath {
			return fmt.Errorf("injected destination write failure")
		}
		return atomicfile.WriteFile(path, data, perm)
	})
	defer restoreWriter()

	_, err := Reclassify(ReclassifyRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectID:    "notes/my-note",
		FilePath:    sourcePath,
		NewTypeName: "book",
		Force:       true,
	})
	if err == nil {
		t.Fatal("expected Reclassify() to fail")
	}

	var svcErr *svcerr.Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != codes.ErrFileWrite {
		t.Fatalf("error code = %s, want %s", svcErr.Code, codes.ErrFileWrite)
	}

	content, readErr := os.ReadFile(sourcePath)
	if readErr != nil {
		t.Fatalf("read source after failure: %v", readErr)
	}
	if !strings.Contains(string(content), "type: note") {
		t.Fatalf("expected source file unchanged after failed move, got:\n%s", string(content))
	}
	if strings.Contains(string(content), "type: book") {
		t.Fatalf("source file was rewritten despite failed move:\n%s", string(content))
	}
}
