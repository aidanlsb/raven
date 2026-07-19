//go:build integration

package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_ObjectLifecycle tests creating, querying, updating, and deleting objects.
func TestIntegration_ObjectLifecycle(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	result := v.RunCLI("new", "person", "Alice", "--field", "email=alice@example.com")
	result.MustSucceed(t)
	v.AssertFileExists("people/alice.md")
	v.AssertFileContains("people/alice.md", "name: Alice")
	v.AssertFileContains("people/alice.md", "email: alice@example.com")

	// Query the person - results are in "items" field
	result = v.RunCLI("query", "type:person")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	// Update the person's email (set uses positional field=value args)
	result = v.RunCLI("set", "people/alice", "email=alice@newdomain.com")
	result.MustSucceed(t)
	v.AssertFileContains("people/alice.md", "email: alice@newdomain.com")

	// Delete the person
	result = v.RunCLI("delete", "people/alice", "--confirm")
	result.MustSucceed(t)
	v.AssertFileNotExists("people/alice.md")
}

func TestIntegration_DeleteJSONSingleAppliesByDefaultWithDryRun(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Delete Preview").MustSucceed(t)
	v.RunCLI("new", "person", "Delete Apply").MustSucceed(t)

	// --dry-run previews without mutating.
	preview := v.RunCLI("delete", "people/delete-preview", "--dry-run")
	preview.MustSucceed(t)
	if preview.Data["preview"] != true {
		t.Fatalf("expected preview response, got: %s", preview.RawJSON)
	}
	if preview.DataString("object_id") != "people/delete-preview" {
		t.Fatalf("expected preview object_id people/delete-preview, got %q", preview.DataString("object_id"))
	}
	v.AssertFileExists("people/delete-preview.md")

	// Default JSON delete applies immediately (no --confirm needed).
	applied := v.RunCLI("delete", "people/delete-apply")
	applied.MustSucceed(t)
	if applied.DataString("deleted") != "people/delete-apply" {
		t.Fatalf("expected deleted object people/delete-apply, got %q", applied.DataString("deleted"))
	}
	v.AssertFileNotExists("people/delete-apply.md")
}

func TestIntegration_EditWithEditsJSON(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-02-15.md", `---
type: page
---
# Daily

- old task
Status: draft
`).
		Build()

	editsJSON := `{"edits":[{"old_str":"- old task","new_str":"- done task"},{"old_str":"Status: draft","new_str":"Status: active"}]}`

	preview := v.RunCLI("edit", "daily/2026-02-15.md", "--edits-json", editsJSON, "--dry-run")
	preview.MustSucceed(t)
	if got := preview.DataString("status"); got != "preview" {
		t.Fatalf("expected preview status, got %q", got)
	}
	if got, ok := preview.Data["count"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected preview count=2, got %#v", preview.Data["count"])
	}
	v.AssertFileContains("daily/2026-02-15.md", "- old task")
	v.AssertFileContains("daily/2026-02-15.md", "Status: draft")

	applied := v.RunCLI("edit", "daily/2026-02-15.md", "--edits-json", editsJSON)
	applied.MustSucceed(t)
	if got := applied.DataString("status"); got != "applied" {
		t.Fatalf("expected applied status, got %q", got)
	}
	if got, ok := applied.Data["count"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected applied count=2, got %#v", applied.Data["count"])
	}
	v.AssertFileContains("daily/2026-02-15.md", "- done task")
	v.AssertFileContains("daily/2026-02-15.md", "Status: active")
	v.AssertFileNotContains("daily/2026-02-15.md", "- old task")
	v.AssertFileNotContains("daily/2026-02-15.md", "Status: draft")
}

func TestIntegration_EditWithEditsJSONIsAtomic(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-02-16.md", `---
type: page
---
# Daily

- old task
Status: draft
`).
		Build()

	editsJSON := `{"edits":[{"old_str":"- old task","new_str":"- done task"},{"old_str":"Status: missing","new_str":"Status: active"}]}`
	result := v.RunCLI("edit", "daily/2026-02-16.md", "--edits-json", editsJSON)
	result.MustFail(t, "STRING_NOT_FOUND")

	v.AssertFileContains("daily/2026-02-16.md", "- old task")
	v.AssertFileContains("daily/2026-02-16.md", "Status: draft")
	v.AssertFileNotContains("daily/2026-02-16.md", "- done task")
}

func TestIntegration_EditSingleModeStillWorks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-02-17.md", `---
type: page
---
# Daily

old task
`).
		Build()

	result := v.RunCLI("edit", "daily/2026-02-17.md", "old task", "done task")
	result.MustSucceed(t)
	v.AssertFileContains("daily/2026-02-17.md", "done task")
	v.AssertFileNotContains("daily/2026-02-17.md", "old task")
}

func TestIntegration_EditRejectsSchemaAndTemplateFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("templates/meeting.md", "# {{title}}\n").
		Build()

	schemaResult := v.RunCLI("edit", "schema.yaml", "version: 1", "version: 2")
	schemaResult.MustFail(t, "VALIDATION_FAILED")
	schemaResult.MustFailWithMessage(t, "rvn schema")
	v.AssertFileContains("schema.yaml", "version: 1")

	templateResult := v.RunCLI("edit", "templates/meeting.md", "{{title}}", "{{name}}")
	templateResult.MustFail(t, "VALIDATION_FAILED")
	templateResult.MustFailWithMessage(t, "rvn template write")
	v.AssertFileContains("templates/meeting.md", "{{title}}")
}

func TestIntegration_TemplateWriteEditUsesEditorContent(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("templates/meeting.md", "# Old Template\n").
		Build()

	editorPath := filepath.Join(t.TempDir(), "fake-editor.sh")
	editorScript := `#!/bin/sh
if ! grep -q "Old Template" "$1"; then
  echo "missing seeded template content" >&2
  exit 7
fi
printf '# Edited Template\n' > "$1"
`
	if err := os.WriteFile(editorPath, []byte(editorScript), 0o755); err != nil {
		t.Fatalf("write fake editor: %v", err)
	}
	configPath := filepath.Join(t.TempDir(), "config.toml")
	configContent := "editor = \"" + strings.ReplaceAll(editorPath, `"`, `\"`) + "\"\neditor_mode = \"terminal\"\n"
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}

	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--config", configPath, "--vault-path", v.Path, "--json", "template", "write", "meeting.md", "--edit")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("template write --edit failed: %v\n%s", err, output)
	}

	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("parse JSON output: %v\n%s", err, output)
	}
	if !resp.OK {
		t.Fatalf("expected ok response, got %s", output)
	}
	if got := resp.Data["status"]; got != "updated" {
		t.Fatalf("status = %#v, want updated", got)
	}
	v.AssertFileContains("templates/meeting.md", "# Edited Template")
	v.AssertFileNotContains("templates/meeting.md", "Old Template")
}

func TestIntegration_TemplateWriteRejectsContentAndEditTogether(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	result := v.RunCLI("template", "write", "meeting.md", "--content", "# Meeting", "--edit")
	result.MustFail(t, "INVALID_INPUT")
	result.MustFailWithMessage(t, "--edit and --content cannot be used together")
}

func TestIntegration_EditRejectsProtectedPrefixAndNonMarkdownFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("protected_prefixes:\n  - private/\n").
		WithFile("private/notes.md", "old task\n").
		WithFile("scratch.txt", "old task\n").
		Build()

	protectedResult := v.RunCLI("edit", "private/notes.md", "old task", "done task")
	protectedResult.MustFail(t, "VALIDATION_FAILED")
	protectedResult.MustFailWithMessage(t, "protected")
	v.AssertFileContains("private/notes.md", "old task")

	nonMarkdownResult := v.RunCLI("edit", "scratch.txt", "old task", "done task")
	nonMarkdownResult.MustFail(t, "VALIDATION_FAILED")
	nonMarkdownResult.MustFailWithMessage(t, "markdown content files")
	v.AssertFileContains("scratch.txt", "old task")
}
