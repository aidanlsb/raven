package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestMoveFileUpdatesBacklinksAfterRename(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "notes/ref" {
		t.Fatalf("UpdatedRefs = %#v, want [notes/ref]", result.UpdatedRefs)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[archive/freya]]") {
		t.Fatalf("backlink not updated, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
}

func TestMoveFileRenameFailureDoesNotRewriteBacklinks(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	destPath := filepath.Join(v.Path, "archive/freya.md")
	if err := os.MkdirAll(destPath, 0o755); err != nil {
		t.Fatalf("mkdir conflicting destination: %v", err)
	}

	_, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   destPath,
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err == nil {
		t.Fatal("expected MoveFile() to fail")
	}

	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if svcErr.Code != ErrorFileWrite {
		t.Fatalf("error code = %s, want %s", svcErr.Code, ErrorFileWrite)
	}

	content := v.ReadFile("notes/ref.md")
	if !strings.Contains(content, "[[people/freya]]") {
		t.Fatalf("backlink changed despite rename failure, content:\n%s", content)
	}
	if _, err := os.Stat(filepath.Join(v.Path, "people/freya.md")); err != nil {
		t.Fatalf("expected source file to remain in place: %v", err)
	}
}

func TestMoveFileWarnsWhenRefRewriteFails(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md", "notes/ref.md")

	if err := os.Remove(filepath.Join(v.Path, "notes/ref.md")); err != nil {
		t.Fatalf("remove backlink source: %v", err)
	}

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.UpdatedRefs) != 0 {
		t.Fatalf("UpdatedRefs = %#v, want empty", result.UpdatedRefs)
	}
	if len(result.WarningMessages) != 1 {
		t.Fatalf("WarningMessages = %#v, want one warning", result.WarningMessages)
	}
	if !strings.Contains(result.WarningMessages[0], "notes/ref") {
		t.Fatalf("warning = %q, want notes/ref context", result.WarningMessages[0])
	}
	if _, err := os.Stat(filepath.Join(v.Path, "archive/freya.md")); err != nil {
		t.Fatalf("expected moved file to exist: %v", err)
	}
}

func TestMoveFileUpdatesSelfRefsAfterRename(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n\nSee [[people/freya]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "people/freya.md")

	result, err := MoveFile(MoveFileRequest{
		VaultPath:         v.Path,
		VaultConfig:       &config.VaultConfig{},
		Schema:            sch,
		SourceFile:        filepath.Join(v.Path, "people/freya.md"),
		DestinationFile:   filepath.Join(v.Path, "archive/freya.md"),
		SourceObjectID:    "people/freya",
		DestinationObject: "archive/freya",
		UpdateRefs:        true,
	})
	if err != nil {
		t.Fatalf("MoveFile() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if len(result.UpdatedRefs) != 1 || result.UpdatedRefs[0] != "archive/freya" {
		t.Fatalf("UpdatedRefs = %#v, want [archive/freya]", result.UpdatedRefs)
	}

	content := v.ReadFile("archive/freya.md")
	if !strings.Contains(content, "[[archive/freya]]") {
		t.Fatalf("self-ref not updated, content:\n%s", content)
	}
}

func indexVaultFiles(t *testing.T, vaultPath string, sch *schema.Schema, relPaths ...string) {
	t.Helper()

	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()

	for _, relPath := range relPaths {
		fullPath := filepath.Join(vaultPath, relPath)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		doc, err := parser.ParseDocument(string(content), fullPath, vaultPath)
		if err != nil {
			t.Fatalf("parse %s: %v", relPath, err)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("index %s: %v", relPath, err)
		}
	}
}
