//go:build integration

package mcp_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func runMCPParityMutationTests(t *testing.T, binary string) {
	t.Run("new", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Person",
			"field": map[string]interface{}{
				"email": "parity@example.com",
			},
		})
		cliResult := vCLI.RunCLI("new", "person", "Parity Person", "--field", "email=parity@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "id", "title", "type"})
	})

	t.Run("upsert", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("upsert", map[string]interface{}{
			"type":    "project",
			"title":   "Parity Project",
			"field":   map[string]interface{}{"status": "active"},
			"content": "# Parity Body",
		})
		cliResult := vCLI.RunCLI("upsert", "project", "Parity Project", "--field", "status=active", "--content", "# Parity Body")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "id", "file", "type", "title"})
	})

	t.Run("upsert_content_file", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		contentFile := filepath.Join(t.TempDir(), "body.md")
		if err := os.WriteFile(contentFile, []byte("# File Body\n\nLong generated content.\n"), 0o644); err != nil {
			t.Fatalf("write content file: %v", err)
		}

		mcpResult := server.callTool("upsert", map[string]interface{}{
			"type":         "project",
			"title":        "File Body Project",
			"field":        map[string]interface{}{"status": "active"},
			"content-file": contentFile,
		})
		cliResult := vCLI.RunCLI("upsert", "project", "File Body Project", "--field", "status=active", "--content-file", contentFile)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "id", "file", "type", "title"})
		vMCP.AssertFileContains("projects/file-body-project.md", "# File Body")
		vCLI.AssertFileContains("projects/file-body-project.md", "# File Body")
	})

	t.Run("add", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Add",
		})
		vCLI.RunCLI("new", "person", "Parity Add").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"text": "Parity add content",
			"to":   "people/parity-add",
		})
		cliResult := vCLI.RunCLI("add", "Parity add content", "--to", "people/parity-add")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "line", "content"})
	})

	t.Run("edit_dry_run", func(t *testing.T) {
		note := `---
type: page
---
# Daily

old task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("daily/2026-02-15.md", note).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("daily/2026-02-15.md", note).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("edit", map[string]interface{}{
			"reference": "daily/2026-02-15.md",
			"old_str":   "old task",
			"new_str":   "done task",
			"dry-run":   true,
		})
		cliResult := vCLI.RunCLI("edit", "daily/2026-02-15.md", "old task", "done task", "--dry-run")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "path", "line", "preview"})
		vMCP.AssertFileContains("daily/2026-02-15.md", "old task")
		vMCP.AssertFileNotContains("daily/2026-02-15.md", "done task")
		vCLI.AssertFileContains("daily/2026-02-15.md", "old task")
		vCLI.AssertFileNotContains("daily/2026-02-15.md", "done task")
	})

	t.Run("edit_applies_by_default", func(t *testing.T) {
		note := `---
type: page
---
# Daily

old task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("daily/2026-02-15.md", note).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("daily/2026-02-15.md", note).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		mcpResult := server.callTool("edit", map[string]interface{}{
			"reference": "daily/2026-02-15.md",
			"old_str":   "old task",
			"new_str":   "done task",
		})
		cliResult := vCLI.RunCLI("edit", "daily/2026-02-15.md", "old task", "done task")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"status", "path", "line"})
		vMCP.AssertFileContains("daily/2026-02-15.md", "done task")
		vCLI.AssertFileContains("daily/2026-02-15.md", "done task")
	})

	t.Run("add_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Bulk Two"})
		vCLI.RunCLI("new", "person", "Add Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Add Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"stdin":      true,
			"object_ids": []interface{}{"people/add-bulk-one", "people/add-bulk-two"},
			"text":       "bulk add preview",
		})
		cliResult := vCLI.RunCLIWithStdin("people/add-bulk-one\npeople/add-bulk-two\n", "add", "--stdin", "bulk add preview")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "content"})
		vMCP.AssertFileNotContains("people/add-bulk-one.md", "bulk add preview")
		vMCP.AssertFileNotContains("people/add-bulk-two.md", "bulk add preview")
		vCLI.AssertFileNotContains("people/add-bulk-one.md", "bulk add preview")
		vCLI.AssertFileNotContains("people/add-bulk-two.md", "bulk add preview")
	})

	t.Run("add_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Add Apply Two"})
		vCLI.RunCLI("new", "person", "Add Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Add Apply Two").MustSucceed(t)

		mcpResult := server.callTool("add", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"object_ids": []interface{}{"people/add-apply-one", "people/add-apply-two"},
			"text":       "bulk add apply",
		})
		cliResult := vCLI.RunCLIWithStdin("people/add-apply-one\npeople/add-apply-two\n", "add", "--stdin", "--confirm", "bulk add apply")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "items", "total", "skipped", "errors", "added", "content"})
	})

	t.Run("set", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Set",
		})
		vCLI.RunCLI("new", "person", "Parity Set").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"reference": "people/parity-set",
			"fields": map[string]interface{}{
				"email": "set@example.com",
			},
		})
		cliResult := vCLI.RunCLI("set", "people/parity-set", "email=set@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"file", "object_id", "type", "updated_fields"})
	})

	t.Run("set_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Bulk Two"})
		vCLI.RunCLI("new", "person", "Set Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Set Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"references": []interface{}{"people/set-bulk-one", "people/set-bulk-two"},
			"fields": map[string]interface{}{
				"email": "bulk@example.com",
			},
		})
		cliResult := vCLI.RunCLIWithStdin("people/set-bulk-one\npeople/set-bulk-two\n", "set", "--stdin", "email=bulk@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "fields"})
		vMCP.AssertFileNotContains("people/set-bulk-one.md", "bulk@example.com")
		vMCP.AssertFileNotContains("people/set-bulk-two.md", "bulk@example.com")
		vCLI.AssertFileNotContains("people/set-bulk-one.md", "bulk@example.com")
		vCLI.AssertFileNotContains("people/set-bulk-two.md", "bulk@example.com")
	})

	t.Run("set_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Set Apply Two"})
		vCLI.RunCLI("new", "person", "Set Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Set Apply Two").MustSucceed(t)

		mcpResult := server.callTool("set", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"references": []interface{}{"people/set-apply-one", "people/set-apply-two"},
			"fields": map[string]interface{}{
				"email": "apply@example.com",
			},
		})
		cliResult := vCLI.RunCLIWithStdin("people/set-apply-one\npeople/set-apply-two\n", "set", "--stdin", "--confirm", "email=apply@example.com")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "items", "total", "skipped", "errors", "modified", "fields"})
	})

	t.Run("update_bulk_preview", func(t *testing.T) {
		taskFile := `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("update", map[string]interface{}{
			"stdin":     true,
			"trait_ids": []interface{}{"tasks/task1.md:trait:1"},
			"value":     "high",
		})
		cliResult := vCLI.RunCLIWithStdin("tasks/task1.md:trait:1\n", "update", "--stdin", "high")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total"})
		vMCP.AssertFileContains("tasks/task1.md", "@priority(low) Second task")
		vMCP.AssertFileNotContains("tasks/task1.md", "@priority(high)")
		vCLI.AssertFileContains("tasks/task1.md", "@priority(low) Second task")
		vCLI.AssertFileNotContains("tasks/task1.md", "@priority(high)")
	})

	t.Run("update_bulk_apply", func(t *testing.T) {
		taskFile := `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("update", map[string]interface{}{
			"stdin":     true,
			"confirm":   true,
			"trait_ids": []interface{}{"tasks/task1.md:trait:1"},
			"value":     "high",
		})
		cliResult := vCLI.RunCLIWithStdin("tasks/task1.md:trait:1\n", "update", "--stdin", "--confirm", "high")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"action", "items", "total", "skipped", "errors", "modified"})
	})

	t.Run("update_bulk_explicit_trait_ids", func(t *testing.T) {
		taskFile := `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("tasks/task1.md", taskFile).
			Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpResult := server.callTool("update", map[string]interface{}{
			"trait_ids": []interface{}{"tasks/task1.md:trait:0", "tasks/task1.md:trait:1"},
			"value":     "high",
		})
		cliResult := vCLI.RunCLI("update",
			"--trait-id", "tasks/task1.md:trait:0",
			"--trait-id", "tasks/task1.md:trait:1",
			"high",
		)

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total"})
		vMCP.AssertFileContains("tasks/task1.md", "@priority(low) First task")
		vMCP.AssertFileContains("tasks/task1.md", "@priority(low) Second task")
		vCLI.AssertFileContains("tasks/task1.md", "@priority(low) First task")
		vCLI.AssertFileContains("tasks/task1.md", "@priority(low) Second task")
	})

	t.Run("section_delete_preview_and_apply", func(t *testing.T) {
		const content = `---
type: project
title: Site
status: active
---

## Alpha

Alpha body

### Child

Child body

## Beta

Beta body
`
		vMCP := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/site.md", content).
			WithFile("notes/ref.md", "See [[projects/site#alpha]] and [[projects/site#child]].\n").
			Build()
		vCLI := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("projects/site.md", content).
			WithFile("notes/ref.md", "See [[projects/site#alpha]] and [[projects/site#child]].\n").
			Build()
		server := newTestServer(t, vMCP.Path, binary)
		server.callTool("reindex", nil)
		vCLI.RunCLI("reindex").MustSucceed(t)

		mcpPreview := server.callTool("section_delete", map[string]interface{}{
			"reference": "projects/site#alpha",
		})
		cliPreview := vCLI.RunCLI("section", "delete", "projects/site#alpha")
		assertEnvelopeParity(t, mcpPreview, cliPreview, []string{
			"section", "file", "line_start", "line_end", "removed_content",
			"deleted_sections", "backlinks", "preview", "status",
		})
		vMCP.AssertFileContains("projects/site.md", "## Alpha")
		vCLI.AssertFileContains("projects/site.md", "## Alpha")

		mcpApplied := server.callTool("section_delete", map[string]interface{}{
			"reference": "projects/site#alpha",
			"confirm":   true,
		})
		cliApplied := vCLI.RunCLI("section", "delete", "projects/site#alpha", "--confirm")
		assertEnvelopeParity(t, mcpApplied, cliApplied, []string{
			"section", "file", "line_start", "line_end", "removed_content",
			"deleted_sections", "backlinks", "status",
		})
		vMCP.AssertFileNotContains("projects/site.md", "## Alpha")
		vMCP.AssertFileContains("projects/site.md", "## Beta")
		vCLI.AssertFileNotContains("projects/site.md", "## Alpha")
		vCLI.AssertFileContains("projects/site.md", "## Beta")
	})

	t.Run("delete", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Parity Delete",
		})
		vCLI.RunCLI("new", "person", "Parity Delete").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"reference": "people/parity-delete",
			"confirm":   true,
		})
		cliResult := vCLI.RunCLI("delete", "people/parity-delete", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"deleted", "behavior", "trash_path"})
	})

	t.Run("delete_single_applies_immediately_parity", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Delete Now",
		})
		vCLI.RunCLI("new", "person", "Delete Now").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"reference": "people/delete-now",
		})
		cliResult := vCLI.RunCLI("delete", "people/delete-now")

		mcpEnv := parseMCPEnvelope(t, mcpResult.Text)
		if !mcpEnv.OK || mcpEnv.Data["deleted"] != "people/delete-now" {
			t.Fatalf("expected MCP delete to apply immediately, got: %s", mcpResult.Text)
		}
		if !cliResult.OK || cliResult.DataString("deleted") != "people/delete-now" {
			t.Fatalf("expected CLI JSON delete to apply immediately, got: %s", cliResult.RawJSON)
		}
		vMCP.AssertFileNotExists("people/delete-now.md")
		vCLI.AssertFileNotExists("people/delete-now.md")
	})

	t.Run("delete_single_dry_run_parity", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{
			"type":  "person",
			"title": "Delete Dry",
		})
		vCLI.RunCLI("new", "person", "Delete Dry").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"reference": "people/delete-dry",
			"dry-run":   true,
		})
		cliResult := vCLI.RunCLI("delete", "people/delete-dry", "--dry-run")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "object_id", "behavior", "backlinks"})
		vMCP.AssertFileExists("people/delete-dry.md")
		vCLI.AssertFileExists("people/delete-dry.md")
	})

	t.Run("delete_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Bulk Two"})
		vCLI.RunCLI("new", "person", "Delete Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Delete Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"stdin":      true,
			"references": []interface{}{"people/delete-bulk-one", "people/delete-bulk-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/delete-bulk-one\npeople/delete-bulk-two\n", "delete", "--stdin")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "behavior"})
		vMCP.AssertFileExists("people/delete-bulk-one.md")
		vMCP.AssertFileExists("people/delete-bulk-two.md")
		vCLI.AssertFileExists("people/delete-bulk-one.md")
		vCLI.AssertFileExists("people/delete-bulk-two.md")
	})

	t.Run("delete_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Delete Apply Two"})
		vCLI.RunCLI("new", "person", "Delete Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Delete Apply Two").MustSucceed(t)

		mcpResult := server.callTool("delete", map[string]interface{}{
			"stdin":      true,
			"confirm":    true,
			"references": []interface{}{"people/delete-apply-one", "people/delete-apply-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/delete-apply-one\npeople/delete-apply-two\n", "delete", "--stdin", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "items", "total", "skipped", "errors", "deleted", "behavior"})
	})

	t.Run("move", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Move Me"})
		server.callTool("new", map[string]interface{}{
			"type":  "project",
			"title": "Move Ref",
			"field": map[string]interface{}{
				"status": "active",
				"owner":  "people/move-me",
			},
		})
		vCLI.RunCLI("new", "person", "Move Me").MustSucceed(t)
		vCLI.RunCLI("new", "project", "Move Ref", "--field", "status=active", "--field", "owner=people/move-me").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"source":      "people/move-me",
			"destination": "archive/move-me-archived",
		})
		cliResult := vCLI.RunCLI("move", "people/move-me", "archive/move-me-archived", "--confirm")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"source", "destination", "updated_refs"})
	})

	t.Run("move_bulk_preview", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Two"})
		vCLI.RunCLI("new", "person", "Bulk One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Bulk Two").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"stdin":       true,
			"destination": "archive/",
			"object_ids":  []interface{}{"people/bulk-one", "people/bulk-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/bulk-one\npeople/bulk-two\n", "move", "--stdin", "archive/")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"preview", "action", "items", "skipped", "total", "warnings", "destination"})
		vMCP.AssertFileExists("people/bulk-one.md")
		vMCP.AssertFileExists("people/bulk-two.md")
		vMCP.AssertFileNotExists("archive/bulk-one.md")
		vMCP.AssertFileNotExists("archive/bulk-two.md")
		vCLI.AssertFileExists("people/bulk-one.md")
		vCLI.AssertFileExists("people/bulk-two.md")
		vCLI.AssertFileNotExists("archive/bulk-one.md")
		vCLI.AssertFileNotExists("archive/bulk-two.md")
	})

	t.Run("move_bulk_apply", func(t *testing.T) {
		vMCP := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		vCLI := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
		server := newTestServer(t, vMCP.Path, binary)

		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Apply One"})
		server.callTool("new", map[string]interface{}{"type": "person", "title": "Bulk Apply Two"})
		vCLI.RunCLI("new", "person", "Bulk Apply One").MustSucceed(t)
		vCLI.RunCLI("new", "person", "Bulk Apply Two").MustSucceed(t)

		mcpResult := server.callTool("move", map[string]interface{}{
			"stdin":       true,
			"confirm":     true,
			"destination": "archive/",
			"object_ids":  []interface{}{"people/bulk-apply-one", "people/bulk-apply-two"},
		})
		cliResult := vCLI.RunCLIWithStdin("people/bulk-apply-one\npeople/bulk-apply-two\n", "move", "--stdin", "--confirm", "archive/")

		assertEnvelopeParity(t, mcpResult, cliResult, []string{"ok", "action", "items", "total", "skipped", "errors", "moved", "destination"})
	})
}
