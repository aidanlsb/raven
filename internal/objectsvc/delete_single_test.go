package objectsvc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestDeleteByReferenceSuccess(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
traits: {}
`)

	filePath := filepath.Join(vaultPath, "people/freya.md")
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Freya\n---\n"), 0o644); err != nil {
		t.Fatalf("seed file: %v", err)
	}

	result, err := DeleteByReference(DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Reference:   "people/freya",
		Behavior:    "trash",
		TrashDir:    ".trash",
	})
	if err != nil {
		t.Fatalf("DeleteByReference: %v", err)
	}
	if result.ObjectID != "people/freya" {
		t.Fatalf("expected deleted object people/freya, got %q", result.ObjectID)
	}
	if result.Behavior != "trash" {
		t.Fatalf("expected behavior trash, got %q", result.Behavior)
	}
	if result.TrashPath == "" {
		t.Fatal("expected trash path to be set")
	}
	if _, err := os.Stat(result.TrashPath); err != nil {
		t.Fatalf("expected trashed file to exist: %v", err)
	}
}

func TestDeleteByReferenceFilePreviewAndApply(t *testing.T) {
	t.Parallel()

	const filePath = "files/images/example/chart.png"
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile(filePath, "png").
		Build()
	sch := loadTestSchema(t, v.Path)

	req := DeleteByReferenceRequest{
		VaultPath:   v.Path,
		VaultConfig: config.DefaultVaultConfig(),
		Schema:      sch,
		Reference:   filePath,
		Behavior:    "trash",
		TrashDir:    ".trash",
	}
	preview, err := PreviewDeleteByReference(req)
	if err != nil {
		t.Fatalf("PreviewDeleteByReference() error = %v", err)
	}
	if preview.ObjectID != filePath {
		t.Fatalf("ObjectID = %q, want %q", preview.ObjectID, filePath)
	}
	if len(preview.Backlinks) != 0 {
		t.Fatalf("Backlinks = %#v, want none for a file path", preview.Backlinks)
	}
	v.AssertFileExists(filePath)

	result, err := DeleteByReference(req)
	if err != nil {
		t.Fatalf("DeleteByReference() error = %v", err)
	}
	v.AssertFileNotExists(filePath)
	v.AssertFileExists(".trash/" + filePath)
	if got := result.ChangeSet.Deleted; len(got) != 1 || got[0] != filePath {
		t.Fatalf("ChangeSet.Deleted = %#v, want [%s]", got, filePath)
	}
}

func TestDeleteByReferenceRejectsSection(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/sectioned.md", "# Notes\n\n## Details\n").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "notes/sectioned.md")

	_, err := PreviewDeleteByReference(DeleteByReferenceRequest{
		VaultPath:   v.Path,
		VaultConfig: config.DefaultVaultConfig(),
		Schema:      sch,
		Reference:   "notes/sectioned#details",
	})
	if err == nil {
		t.Fatal("PreviewDeleteByReference() succeeded for a section ID")
	}
	var serviceErr *svcerr.Error
	if !errors.As(err, &serviceErr) || serviceErr.Code != codes.ErrInvalidInput {
		t.Fatalf("error = %v, want %s", err, codes.ErrInvalidInput)
	}
	v.AssertFileExists("notes/sectioned.md")
}
