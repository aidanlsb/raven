package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// createTestVaultForReclassify creates a temp vault with schema and a typed file.
// Returns the vault path.
func createTestVaultForReclassify(t *testing.T, schemaYAML, fileName, fileContent string) string {
	t.Helper()
	vaultPath := t.TempDir()

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	dir := filepath.Dir(filepath.Join(vaultPath, fileName))
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(vaultPath, fileName), []byte(fileContent), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	return vaultPath
}

func setupReclassifyGlobals(t *testing.T, vaultPath string) {
	t.Helper()
	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := reclassifyFieldFlags
	prevNoMove := reclassifyNoMove
	prevUpdateRefs := reclassifyUpdateRefs
	prevForce := reclassifyForce
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		reclassifyFieldFlags = prevFields
		reclassifyNoMove = prevNoMove
		reclassifyUpdateRefs = prevUpdateRefs
		reclassifyForce = prevForce
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	reclassifyFieldFlags = nil
	reclassifyNoMove = false
	reclassifyUpdateRefs = true
	reclassifyForce = false
}

func TestReclassifyBasicTypeChange(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
    fields:
      title:
        type: string
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nSome content.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyNoMove = true // don't move for this test
	reclassifyForce = true  // skip dropped fields confirmation

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if resp.Data.OldType != "note" {
		t.Fatalf("expected old_type=note, got %s", resp.Data.OldType)
	}
	if resp.Data.NewType != "book" {
		t.Fatalf("expected new_type=book, got %s", resp.Data.NewType)
	}

	// Verify the file was updated
	b, err := os.ReadFile(filepath.Join(vaultPath, "notes/my-note.md"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "type: book") {
		t.Fatalf("expected type: book in frontmatter, got:\n%s", content)
	}
	if !strings.Contains(content, "Some content.") {
		t.Fatalf("expected body content preserved, got:\n%s", content)
	}
}

func TestReclassifyAutoMoveToDefaultPath(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyForce = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if !resp.Data.Moved {
		t.Fatalf("expected moved=true; out=%s", out)
	}
	if resp.Data.NewPath != "books/my-note.md" {
		t.Fatalf("expected new_path=books/my-note.md, got %s", resp.Data.NewPath)
	}

	// Verify old file is gone
	if _, err := os.Stat(filepath.Join(vaultPath, "notes/my-note.md")); !os.IsNotExist(err) {
		t.Fatalf("expected old file to be removed")
	}

	// Verify new file exists
	if _, err := os.Stat(filepath.Join(vaultPath, "books/my-note.md")); err != nil {
		t.Fatalf("expected new file to exist: %v", err)
	}
}

func TestReclassifyNoMoveSkipsMoving(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyNoMove = true
	reclassifyForce = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if resp.Data.Moved {
		t.Fatalf("expected moved=false when --no-move is set")
	}

	// File should still be in the original location
	if _, err := os.Stat(filepath.Join(vaultPath, "notes/my-note.md")); err != nil {
		t.Fatalf("expected file to remain at original path: %v", err)
	}
}

func TestReclassifyMissingRequiredFieldsJSON(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
    fields:
      author:
        type: string
        required: true
      genre:
        type: string
        required: true
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code    string                 `json:"code"`
			Details map[string]interface{} `json:"details"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrRequiredField {
		t.Fatalf("expected error code %s, got %v; out=%s", ErrRequiredField, resp.Error, out)
	}
	if resp.Error.Details["retry_with"] == nil {
		t.Fatalf("expected retry_with in details; out=%s", out)
	}
}

func TestReclassifyMissingFieldsSuppliedViaFlag(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
    fields:
      author:
        type: string
        required: true
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyFieldFlags = []string{"author=Tolkien"}
	reclassifyNoMove = true
	reclassifyForce = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}

	// Verify field was added
	b, err := os.ReadFile(filepath.Join(vaultPath, "notes/my-note.md"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "author: Tolkien") {
		t.Fatalf("expected author field in frontmatter, got:\n%s", content)
	}
}

func TestReclassifyDefaultValuesApplied(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
  book:
    default_path: books/
    fields:
      status:
        type: string
        required: true
        default: draft
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyNoMove = true
	reclassifyForce = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}

	// Verify default was applied
	b, err := os.ReadFile(filepath.Join(vaultPath, "notes/my-note.md"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "status: draft") {
		t.Fatalf("expected status: draft in frontmatter, got:\n%s", content)
	}

	// Check added_fields in result
	found := false
	for _, f := range resp.Data.AddedFields {
		if f == "status" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected 'status' in added_fields, got %v", resp.Data.AddedFields)
	}
}

func TestReclassifyDroppedFieldsConfirmation(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
    fields:
      category:
        type: string
      priority:
        type: string
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\ncategory: tech\npriority: high\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true (confirmation needed); out=%s", out)
	}
	if !resp.Data.NeedsConfirm {
		t.Fatalf("expected needs_confirm=true; out=%s", out)
	}
	if len(resp.Data.DroppedFields) == 0 {
		t.Fatalf("expected dropped_fields to be non-empty; out=%s", out)
	}
}

func TestReclassifyForceSkipsDroppedConfirmation(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
    fields:
      category:
        type: string
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\ncategory: tech\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyForce = true
	reclassifyNoMove = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}
	if resp.Data.NeedsConfirm {
		t.Fatalf("expected needs_confirm=false when --force is set")
	}

	// Verify the dropped field was removed
	b, err := os.ReadFile(filepath.Join(vaultPath, "notes/my-note.md"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if strings.Contains(content, "category") {
		t.Fatalf("expected category to be dropped, got:\n%s", content)
	}
}

func TestReclassifySameTypeError(t *testing.T) {
	schemaYAML := `version: 2
types:
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"books/my-book.md",
		"---\ntype: book\ntitle: My Book\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "books/my-book", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error code %s, got %v; out=%s", ErrInvalidInput, resp.Error, out)
	}
}

func TestReclassifyBuiltinTypeError(t *testing.T) {
	schemaYAML := `version: 2
types:
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"books/my-book.md",
		"---\ntype: book\ntitle: My Book\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "books/my-book", "page"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("expected error code %s, got %v; out=%s", ErrInvalidInput, resp.Error, out)
	}
}

func TestReclassifyUnknownTypeError(t *testing.T) {
	schemaYAML := `version: 2
types:
  book:
    default_path: books/
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"books/my-book.md",
		"---\ntype: book\ntitle: My Book\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "books/my-book", "unicorn"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK    bool `json:"ok"`
		Error *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false; out=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrTypeNotFound {
		t.Fatalf("expected error code %s, got %v; out=%s", ErrTypeNotFound, resp.Error, out)
	}
}

func TestReclassifyTitleFieldPreserved(t *testing.T) {
	schemaYAML := `version: 2
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
  book:
    default_path: books/
    fields:
      title:
        type: string
`
	vaultPath := createTestVaultForReclassify(t, schemaYAML,
		"notes/my-note.md",
		"---\ntype: note\ntitle: My Note\n---\n\nContent.\n")

	setupReclassifyGlobals(t, vaultPath)
	reclassifyNoMove = true
	reclassifyForce = true

	out := captureStdout(t, func() {
		if err := runReclassify(vaultPath, "notes/my-note", "book"); err != nil {
			t.Fatalf("runReclassify: %v", err)
		}
	})

	var resp struct {
		OK   bool             `json:"ok"`
		Data ReclassifyResult `json:"data"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("parse JSON: %v; out=%s", err, out)
	}
	if !resp.OK {
		t.Fatalf("expected ok=true; out=%s", out)
	}

	// Title should be preserved (exists on both types)
	b, err := os.ReadFile(filepath.Join(vaultPath, "notes/my-note.md"))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	content := string(b)
	if !strings.Contains(content, "title: My Note") {
		t.Fatalf("expected title preserved, got:\n%s", content)
	}
	if !strings.Contains(content, "type: book") {
		t.Fatalf("expected type: book, got:\n%s", content)
	}
}
