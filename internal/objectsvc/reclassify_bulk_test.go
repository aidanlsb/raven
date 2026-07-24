package objectsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
)

func TestReclassifyBulkPreviewAndApplyMixedResults(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
      legacy:
        type: string
  doc:
    default_path: docs/
    fields:
      title:
        type: string
        required: true
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)
	if err := os.MkdirAll(filepath.Join(vaultPath, "notes"), 0o755); err != nil {
		t.Fatalf("mkdir notes: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(vaultPath, "notes/ready.md"),
		[]byte("---\ntype: note\ntitle: Ready\nlegacy: remove me\n---\n\nReady body.\n"),
		0o644,
	); err != nil {
		t.Fatalf("write ready: %v", err)
	}
	if err := os.WriteFile(
		filepath.Join(vaultPath, "notes/missing.md"),
		[]byte("---\ntype: note\n---\n\nMissing title.\n"),
		0o644,
	); err != nil {
		t.Fatalf("write missing: %v", err)
	}

	request := ReclassifyBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectIDs:   []string{"notes/ready", "notes/missing"},
		NewTypeName: "doc",
		UpdateRefs:  true,
	}

	preview, err := PreviewReclassifyBulk(request)
	if err != nil {
		t.Fatalf("PreviewReclassifyBulk: %v", err)
	}
	if len(preview.Items) != 1 || len(preview.Skipped) != 1 {
		t.Fatalf("preview items/skipped = %d/%d, want 1/1: %#v", len(preview.Items), len(preview.Skipped), preview)
	}
	item := preview.Items[0]
	if !item.Moved || item.NewPath != "docs/ready.md" {
		t.Fatalf("preview move = %#v, want docs/ready.md", item)
	}
	if !item.NeedsConfirm || len(item.DroppedFields) != 1 || item.DroppedFields[0] != "legacy" {
		t.Fatalf("preview dropped fields = %#v, want legacy requiring force", item)
	}
	if item.UpdatedRefs == nil || item.AddedFields == nil {
		t.Fatalf("preview arrays must be non-nil: %#v", item)
	}
	failure := preview.Skipped[0]
	if failure.ErrorCode != codes.ErrRequiredFieldMissing {
		t.Fatalf("preview error code = %q, want %q", failure.ErrorCode, codes.ErrRequiredFieldMissing)
	}
	if failure.ErrorDetails["missing_fields"] == nil {
		t.Fatalf("preview missing required-field details: %#v", failure)
	}
	assertReclassifyType(t, filepath.Join(vaultPath, "notes/ready.md"), "note")
	if _, err := os.Stat(filepath.Join(vaultPath, "docs/ready.md")); !os.IsNotExist(err) {
		t.Fatalf("preview created destination, err=%v", err)
	}

	blocked, err := ApplyReclassifyBulk(request, nil)
	if err != nil {
		t.Fatalf("ApplyReclassifyBulk without force: %v", err)
	}
	if blocked.Reclassified != 0 || blocked.Skipped != 1 || blocked.Errors != 1 {
		t.Fatalf("blocked summary = %#v, want 0 reclassified, 1 skipped, 1 error", blocked)
	}
	assertReclassifyType(t, filepath.Join(vaultPath, "notes/ready.md"), "note")

	request.Force = true
	applied, err := ApplyReclassifyBulk(request, nil)
	if err != nil {
		t.Fatalf("ApplyReclassifyBulk with force: %v", err)
	}
	if applied.Reclassified != 1 || applied.Skipped != 0 || applied.Errors != 1 {
		t.Fatalf("applied summary = %#v, want 1 reclassified, 0 skipped, 1 error", applied)
	}
	if len(applied.Results) != 2 {
		t.Fatalf("applied results len = %d, want 2", len(applied.Results))
	}
	assertReclassifyType(t, filepath.Join(vaultPath, "docs/ready.md"), "doc")
	assertReclassifyType(t, filepath.Join(vaultPath, "notes/missing.md"), "note")
}

func TestReclassifyBulkSkipsDuplicateDestinations(t *testing.T) {
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
  doc:
    default_path: docs/
    fields:
      title:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)
	for _, relPath := range []string{"notes/shared.md", "archive/shared.md"} {
		filePath := filepath.Join(vaultPath, relPath)
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", relPath, err)
		}
		if err := os.WriteFile(filePath, []byte("---\ntype: note\ntitle: Shared\n---\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", relPath, err)
		}
	}

	request := ReclassifyBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectIDs:   []string{"notes/shared", "archive/shared"},
		NewTypeName: "doc",
		UpdateRefs:  true,
	}

	preview, err := PreviewReclassifyBulk(request)
	if err != nil {
		t.Fatalf("PreviewReclassifyBulk: %v", err)
	}
	if len(preview.Items) != 1 || len(preview.Skipped) != 1 {
		t.Fatalf("preview items/skipped = %d/%d, want 1/1: %#v", len(preview.Items), len(preview.Skipped), preview)
	}
	if got := preview.Skipped[0].ErrorDetails["destination"]; got != "docs/shared.md" {
		t.Fatalf("collision destination = %#v, want docs/shared.md", got)
	}

	applied, err := ApplyReclassifyBulk(request, nil)
	if err != nil {
		t.Fatalf("ApplyReclassifyBulk: %v", err)
	}
	if applied.Reclassified != 1 || applied.Skipped != 1 || applied.Errors != 0 {
		t.Fatalf("applied summary = %#v, want 1 reclassified and 1 skipped", applied)
	}
	assertReclassifyType(t, filepath.Join(vaultPath, "docs/shared.md"), "doc")
	assertReclassifyType(t, filepath.Join(vaultPath, "archive/shared.md"), "note")
}

func assertReclassifyType(t *testing.T, filePath, objectType string) {
	t.Helper()
	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}
	want := "type: " + objectType
	if !containsLine(string(content), want) {
		t.Fatalf("%s does not contain %q:\n%s", filePath, want, string(content))
	}
}

func containsLine(content, want string) bool {
	for _, line := range strings.Split(content, "\n") {
		if line == want {
			return true
		}
	}
	return false
}
