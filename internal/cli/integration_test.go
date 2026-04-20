//go:build integration

package cli_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestIntegration_DeleteJSONSinglePreviewRequiresConfirm(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Delete Preview").MustSucceed(t)

	preview := v.RunCLI("delete", "people/delete-preview")
	preview.MustSucceed(t)
	if preview.Data["preview"] != true {
		t.Fatalf("expected preview response, got: %s", preview.RawJSON)
	}
	if preview.DataString("object_id") != "people/delete-preview" {
		t.Fatalf("expected preview object_id people/delete-preview, got %q", preview.DataString("object_id"))
	}
	v.AssertFileExists("people/delete-preview.md")

	confirm := v.RunCLI("delete", "people/delete-preview", "--confirm")
	confirm.MustSucceed(t)
	if confirm.DataString("deleted") != "people/delete-preview" {
		t.Fatalf("expected deleted object people/delete-preview, got %q", confirm.DataString("deleted"))
	}
	v.AssertFileNotExists("people/delete-preview.md")
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

	preview := v.RunCLI("edit", "daily/2026-02-15.md", "--edits-json", editsJSON)
	preview.MustSucceed(t)
	if got := preview.DataString("status"); got != "preview" {
		t.Fatalf("expected preview status, got %q", got)
	}
	if got, ok := preview.Data["count"].(float64); !ok || int(got) != 2 {
		t.Fatalf("expected preview count=2, got %#v", preview.Data["count"])
	}
	v.AssertFileContains("daily/2026-02-15.md", "- old task")
	v.AssertFileContains("daily/2026-02-15.md", "Status: draft")

	applied := v.RunCLI("edit", "daily/2026-02-15.md", "--edits-json", editsJSON, "--confirm")
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
	result := v.RunCLI("edit", "daily/2026-02-16.md", "--edits-json", editsJSON, "--confirm")
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

	result := v.RunCLI("edit", "daily/2026-02-17.md", "old task", "done task", "--confirm")
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

	schemaResult := v.RunCLI("edit", "schema.yaml", "version: 1", "version: 2", "--confirm")
	schemaResult.MustFail(t, "VALIDATION_FAILED")
	schemaResult.MustFailWithMessage(t, "rvn schema")
	v.AssertFileContains("schema.yaml", "version: 1")

	templateResult := v.RunCLI("edit", "templates/meeting.md", "{{title}}", "{{name}}", "--confirm")
	templateResult.MustFail(t, "VALIDATION_FAILED")
	templateResult.MustFailWithMessage(t, "rvn template write")
	v.AssertFileContains("templates/meeting.md", "{{title}}")
}

func TestIntegration_EditRejectsProtectedPrefixAndNonMarkdownFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("protected_prefixes:\n  - private/\n").
		WithFile("private/notes.md", "old task\n").
		WithFile("scratch.txt", "old task\n").
		Build()

	protectedResult := v.RunCLI("edit", "private/notes.md", "old task", "done task", "--confirm")
	protectedResult.MustFail(t, "VALIDATION_FAILED")
	protectedResult.MustFailWithMessage(t, "protected")
	v.AssertFileContains("private/notes.md", "old task")

	nonMarkdownResult := v.RunCLI("edit", "scratch.txt", "old task", "done task", "--confirm")
	nonMarkdownResult.MustFail(t, "VALIDATION_FAILED")
	nonMarkdownResult.MustFailWithMessage(t, "markdown content files")
	v.AssertFileContains("scratch.txt", "old task")
}

func TestIntegration_ProtectedPrefixesRejectMutationCommands(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      title:
        type: string
      status:
        type: enum
        values: [active, done]
traits:
  todo:
    type: enum
    values: [open, done]
`).
		WithRavenYAML("protected_prefixes:\n  - private/\n").
		WithFile("private/task.md", `---
type: project
title: Protected Task
status: active
---
- task @todo(open)
`).
		WithFile("private/notes.md", "# Notes\n").
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	newResult := v.RunCLI("new", "project", "Blocked Project", "--path", "private/blocked-project")
	newResult.MustFail(t, "VALIDATION_FAILED")
	newResult.MustFailWithMessage(t, "protected")

	upsertResult := v.RunCLI("upsert", "project", "Blocked Project", "--path", "private/blocked-project", "--content", "# blocked")
	upsertResult.MustFail(t, "VALIDATION_FAILED")
	upsertResult.MustFailWithMessage(t, "protected")

	addResult := v.RunCLI("add", "Protected note", "--to", "private/notes.md")
	addResult.MustFail(t, "VALIDATION_FAILED")
	addResult.MustFailWithMessage(t, "protected")
	v.AssertFileNotContains("private/notes.md", "Protected note")

	setResult := v.RunCLI("set", "private/task.md", "status=done")
	setResult.MustFail(t, "VALIDATION_FAILED")
	setResult.MustFailWithMessage(t, "protected")
	v.AssertFileContains("private/task.md", "status: active")

	updateResult := v.RunCLI("update", "private/task.md:trait:0", "done")
	updateResult.MustFail(t, "VALIDATION_FAILED")
	updateResult.MustFailWithMessage(t, "protected")
	v.AssertFileContains("private/task.md", "@todo(open)")

	moveResult := v.RunCLI("move", "private/task.md", "archive/protected-task.md")
	moveResult.MustFail(t, "VALIDATION_FAILED")
	moveResult.MustFailWithMessage(t, "protected")
	v.AssertFileExists("private/task.md")

	deleteResult := v.RunCLI("delete", "private/task.md", "--confirm")
	deleteResult.MustFail(t, "VALIDATION_FAILED")
	deleteResult.MustFailWithMessage(t, "protected")
	v.AssertFileExists("private/task.md")
}

func TestIntegration_MoveRejectsProtectedBacklinkUpdates(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  project:
    fields:
      title:
        type: string
`).
		WithRavenYAML("protected_prefixes:\n  - private/\n").
		WithFile("projects/open.md", `---
type: project
title: Open
---
`).
		WithFile("private/ref.md", `See [[projects/open]] later.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("move", "projects/open.md", "archive/open.md")
	result.MustFail(t, "VALIDATION_FAILED")
	result.MustFailWithMessage(t, "protected")
	v.AssertFileExists("projects/open.md")
	v.AssertFileNotExists("archive/open.md")
	v.AssertFileContains("private/ref.md", "[[projects/open]]")
}
func TestIntegration_InitReturnsPostInitGuidance(t *testing.T) {
	t.Parallel()
	binary := testutil.BuildCLI(t)
	root := t.TempDir()
	configFile := filepath.Join(root, "config.toml")
	stateFile := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "New Notes")

	cmd := exec.Command(binary, "--config", configFile, "--state", stateFile, "--json", "init", vaultPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init failed: %v\n%s", err, output)
	}

	var resp struct {
		OK   bool                   `json:"ok"`
		Data map[string]interface{} `json:"data"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal init response: %v\n%s", err, output)
	}
	if !resp.OK {
		t.Fatalf("expected init success, got: %s", output)
	}

	postInit, ok := resp.Data["post_init"].(map[string]interface{})
	if !ok {
		t.Fatalf("post_init = %#v, want map", resp.Data["post_init"])
	}
	if got := postInit["suggested_name"]; got != "new-notes" {
		t.Fatalf("suggested_name = %#v, want %q", got, "new-notes")
	}
	if got := postInit["already_registered"]; got != false {
		t.Fatalf("already_registered = %#v, want false", got)
	}
	commands, ok := postInit["commands"].(map[string]interface{})
	if !ok {
		t.Fatalf("commands = %#v, want map", postInit["commands"])
	}
	if _, ok := commands["register"]; !ok {
		t.Fatalf("expected register command in %#v", commands)
	}
}

func TestIntegration_JSONPreRunMissingVaultReturnsEnvelope(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)
	missingVault := filepath.Join(t.TempDir(), "missing-vault")

	cmd := exec.Command(binary, "--vault-path", missingVault, "--json", "query", "type:project")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected JSON envelope without process failure: %v\n%s", err, output)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code       string `json:"code"`
			Message    string `json:"message"`
			Suggestion string `json:"suggestion"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal prerun response: %v\n%s", err, output)
	}
	if resp.OK {
		t.Fatalf("expected failure envelope, got success: %s", output)
	}
	if resp.Error.Code != "VAULT_NOT_FOUND" {
		t.Fatalf("error.code=%q, want VAULT_NOT_FOUND\n%s", resp.Error.Code, output)
	}
	if !strings.Contains(resp.Error.Message, missingVault) {
		t.Fatalf("message=%q, want vault path", resp.Error.Message)
	}
	if !strings.Contains(resp.Error.Suggestion, "rvn init") {
		t.Fatalf("suggestion=%q, want init hint", resp.Error.Suggestion)
	}
}

func TestIntegration_JSONPreRunConfigFailureReturnsEnvelope(t *testing.T) {
	t.Parallel()

	binary := testutil.BuildCLI(t)
	dir := t.TempDir()
	configFile := filepath.Join(dir, "broken.toml")
	if err := os.WriteFile(configFile, []byte("not = [valid"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	cmd := exec.Command(binary, "--config", configFile, "--json", "version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("expected JSON envelope without process failure: %v\n%s", err, output)
	}

	var resp struct {
		OK    bool `json:"ok"`
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(output, &resp); err != nil {
		t.Fatalf("unmarshal config response: %v\n%s", err, output)
	}
	if resp.OK {
		t.Fatalf("expected failure envelope, got success: %s", output)
	}
	if resp.Error.Code != "CONFIG_INVALID" {
		t.Fatalf("error.code=%q, want CONFIG_INVALID\n%s", resp.Error.Code, output)
	}
	if !strings.Contains(resp.Error.Message, "failed to load config") {
		t.Fatalf("message=%q, want config failure", resp.Error.Message)
	}
}

// TestIntegration_QueryByField tests querying objects by field values.
func TestIntegration_QueryByField(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects with different statuses
	v.RunCLI("new", "project", "Project Alpha", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Beta", "--field", "status=paused").MustSucceed(t)
	v.RunCLI("new", "project", "Project Gamma", "--field", "status=active").MustSucceed(t)

	// Query for active projects - uses == for equality
	result := v.RunCLI("query", "type:project .status==active")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 2)

	// Query for paused projects
	result = v.RunCLI("query", "type:project .status==paused")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

func TestIntegration_QueryRefreshRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  project:
    default_path: projects/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: enum
        values: [active, done]
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("objects/projects/weekly.md", `---
type: project
title: Weekly
status: active
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	updated := `---
type: project
title: Weekly
status: done
---
`
	filePath := filepath.Join(v.Path, "objects/projects/weekly.md")
	if err := os.WriteFile(filePath, []byte(updated), 0o644); err != nil {
		t.Fatalf("failed to update project file: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(filePath, future, future); err != nil {
		t.Fatalf("failed to bump project mtime: %v", err)
	}

	result := v.RunCLI("query", "type:project", "--refresh")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	item, ok := result.DataList("items")[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected query item map, got %#v", result.DataList("items")[0])
	}
	if got := item["id"]; got != "projects/weekly" {
		t.Fatalf("expected refreshed project ID projects/weekly, got %#v", got)
	}
	fields, ok := item["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map, got %#v", item["fields"])
	}
	if got := fields["status"]; got != "done" {
		t.Fatalf("expected refreshed status done, got %#v", got)
	}
}

func TestIntegration_QueryRefreshRemovesDeletedFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)
	v.AssertQueryCount("type:person", 1)

	if err := os.Remove(filepath.Join(v.Path, "people/alice.md")); err != nil {
		t.Fatalf("failed to remove person file: %v", err)
	}

	result := v.RunCLI("query", "type:person", "--refresh")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 0)
}

func TestIntegration_QueryFailsOnSchemaLoadError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Schema Query").MustSucceed(t)

	schemaPath := filepath.Join(v.Path, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte("version: ["), 0o644); err != nil {
		t.Fatalf("failed to corrupt schema for test: %v", err)
	}

	result := v.RunCLI("query", "type:project")
	result.MustFail(t, "SCHEMA_INVALID")
	result.MustFailWithMessage(t, "Fix schema.yaml and try again")
}

func TestIntegration_QueryAmbiguousReferenceReturnsQueryInvalid(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Alex").MustSucceed(t)
	v.RunCLI("new", "person", "Alex").MustSucceed(t)

	result := v.RunCLI("query", "type:project refs([[alex]])")
	result.MustFail(t, "QUERY_INVALID")
	result.MustFailWithMessage(t, "Use a full object ID/path to disambiguate")
}

// TestIntegration_ReferencesAndBacklinks tests reference resolution and backlinks.
func TestIntegration_ReferencesAndBacklinks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Create a project owned by Alice
	v.RunCLI("new", "project", "Website Redesign", "--field", "status=active", "--field", "owner=[[people/alice]]").MustSucceed(t)

	// Check that Alice has a backlink from the project
	v.AssertBacklinks("people/alice", 1)

	// Query for projects that reference Alice - results are in "items" field
	result := v.RunCLI("query", "type:project refs([[people/alice]])")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

// TestIntegration_TraitQueries tests trait queries with various predicates.
func TestIntegration_TraitQueries(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

@due(2024-01-01) Important task from the past
`).
		WithFile("tasks/task2.md", `---
type: page
---
# Task 2

@priority(high) High priority task
`).
		Build()

	// Reindex to pick up the files
	v.RunCLI("reindex").MustSucceed(t)

	// Query for due traits - results are in "items" field
	result := v.RunCLI("query", "trait:due")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	// Query for priority traits - uses == for equality
	result = v.RunCLI("query", "trait:priority .value==high")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

// TestIntegration_MoveWithReferenceUpdate tests that moving files updates references.
func TestIntegration_MoveWithReferenceUpdate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Create a project that references Alice
	v.RunCLI("new", "project", "Website", "--field", "owner=[[people/alice]]").MustSucceed(t)

	// Move Alice within the people directory (rename)
	result := v.RunCLI("move", "people/alice", "people/alice-archived")
	result.MustSucceed(t)

	// Verify the move
	v.AssertFileNotExists("people/alice.md")
	v.AssertFileExists("people/alice-archived.md")

	// Verify the reference was updated in the project
	v.AssertFileContains("projects/website.md", "[[people/alice-archived]]")
}

// TestIntegration_MoveWithReferenceUpdate_BareFrontmatterRef verifies that
// schema-typed ref fields written as bare YAML strings are also updated.
func TestIntegration_MoveWithReferenceUpdate_BareFrontmatterRef(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person.
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Create a project with a bare frontmatter ref (not [[wikilink]] syntax).
	v.RunCLI("new", "project", "Website", "--field", "owner=people/alice").MustSucceed(t)

	// Move Alice within the people directory (rename).
	result := v.RunCLI("move", "people/alice", "people/alice-archived")
	result.MustSucceed(t)

	// Verify the move happened.
	v.AssertFileNotExists("people/alice.md")
	v.AssertFileExists("people/alice-archived.md")

	// The bare frontmatter ref should be rewritten too.
	v.AssertFileContains("projects/website.md", "owner: people/alice-archived")
}

// TestIntegration_MoveWithShortSourceReference ensures source refs are resolved
// before backlink/index updates (e.g. `rvn move alice people/alice-archived`).
func TestIntegration_MoveWithShortSourceReference(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Alice").MustSucceed(t)
	v.RunCLI("new", "project", "Website", "--field", "owner=[[people/alice]]").MustSucceed(t)

	// Move using short reference as source.
	result := v.RunCLI("move", "alice", "people/alice-archived")
	result.MustSucceed(t)

	v.AssertFileNotExists("people/alice.md")
	v.AssertFileExists("people/alice-archived.md")
	v.AssertFileContains("projects/website.md", "[[people/alice-archived]]")
}

// TestIntegration_MoveDirectoryDestinationUsesSourceFilename verifies that
// single-object move to a directory destination (trailing slash) derives the
// destination filename from the source object.
func TestIntegration_MoveDirectoryDestinationUsesSourceFilename(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("objects/spec/raven-move-friction.md", `---
type: note
---
`).
		Build()

	// Source is not in the default path and should resolve via short object ID.
	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("move", "spec/raven-move-friction", "note/")
	result.MustSucceed(t)

	v.AssertFileNotExists("objects/spec/raven-move-friction.md")
	v.AssertFileExists("objects/note/raven-move-friction.md")
	if got := result.DataString("destination"); got != "note/raven-move-friction" {
		t.Fatalf("expected destination object ID %q, got %q", "note/raven-move-friction", got)
	}
}

// TestIntegration_MoveDestinationWithRootPrefixAvoidsDoubleRoot verifies that
// destinations already including the object root are not prefixed twice.
func TestIntegration_MoveDestinationWithRootPrefixAvoidsDoubleRoot(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  doc:
    default_path: doc/
`).
		WithRavenYAML(`directories:
  type: objects/
  page: pages/
`).
		WithFile("objects/doc/test-note.md", `---
type: doc
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("move", "doc/test-note", "objects/doc/test-note-moved")
	result.MustSucceed(t)

	v.AssertFileNotExists("objects/doc/test-note.md")
	v.AssertFileExists("objects/doc/test-note-moved.md")
	v.AssertFileNotExists("objects/objects/doc/test-note-moved.md")

	if got := result.DataString("destination"); got != "doc/test-note-moved" {
		t.Fatalf("expected destination object ID %q, got %q", "doc/test-note-moved", got)
	}
}

func TestIntegration_NewWithExplicitPath(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLI("new", "note", "Raven Friction", "--path", "note/raven-logo-brief")
	result.MustSucceed(t)

	v.AssertFileExists("objects/note/raven-logo-brief.md")
	v.AssertFileNotExists("objects/note/raven-friction.md")
}

// TestIntegration_SchemaValidationErrors tests that schema validation errors are properly reported.
func TestIntegration_SchemaValidationErrors(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Try to create a person without providing the required fields via title
	// Create the file manually without a required field and verify check finds the issue
	v.RunCLI("new", "person", "TestPerson").MustSucceed(t)

	// Unknown fields should fail fast.
	result := v.RunCLI("set", "people/testperson", "status=invalid_value")
	result.MustFail(t, "UNKNOWN_FIELD")
	if result.Error == nil || result.Error.Details == nil {
		t.Fatalf("expected unknown field details in error, got: %#v", result.Error)
	}
	unknownFieldsRaw, ok := result.Error.Details["unknown_fields"].([]interface{})
	if !ok || len(unknownFieldsRaw) == 0 {
		t.Fatalf("expected unknown_fields in details, got: %#v", result.Error.Details)
	}
	if unknownFieldsRaw[0] != "status" {
		t.Fatalf("expected unknown field 'status', got: %#v", unknownFieldsRaw)
	}
	result.MustFailWithMessage(t, "schema type person")
}

func TestIntegration_SetBulkFailsOnSchemaLoadError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Schema Broken").MustSucceed(t)

	schemaPath := filepath.Join(v.Path, "schema.yaml")
	if err := os.WriteFile(schemaPath, []byte("version: ["), 0o644); err != nil {
		t.Fatalf("failed to corrupt schema for test: %v", err)
	}

	result := v.RunCLIWithStdin("people/schema-broken\n", "set", "--stdin", "email=broken@example.com", "--confirm")
	result.MustFail(t, "SCHEMA_INVALID")
	result.MustFailWithMessage(t, "Fix schema.yaml and try again")
}

func TestIntegration_SetResolvesObjectIDsAndWikiLinks(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	v.RunCLI("new", "person", "Dana").MustSucceed(t)

	v.RunCLI("set", "people/dana", "email=dana@example.com").MustSucceed(t)
	v.RunCLI("set", "[[people/dana]]", "email=dana+wiki@example.com").MustSucceed(t)

	v.AssertFileContains("objects/people/dana.md", "email: dana+wiki@example.com")
}

func TestIntegration_SetValidatesTypedValuesAtWriteTime(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Dana").MustSucceed(t)

	// email is a string field; unquoted true should fail type validation.
	result := v.RunCLI("set", "people/dana", "email=true")
	result.MustFail(t, "VALIDATION_FAILED")

	// Confirm no invalid bool value was written into frontmatter.
	v.AssertFileNotContains("people/dana.md", "email: true")
}

func TestIntegration_UpsertValidatesTypedValuesAtWriteTime(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "project", "Website", "--field", "status=active").MustSucceed(t)

	// status is enum(active|paused|done); invalid enum should fail.
	result := v.RunCLI("upsert", "project", "Website", "--field", "status=not-a-valid-status")
	result.MustFail(t, "VALIDATION_FAILED")

	// Existing value should remain unchanged.
	v.AssertFileContains("projects/website.md", "status: active")
}

func TestIntegration_UpsertUnknownFieldFailsFast(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("upsert", "person", "Unknown Field User", "--field", "favorite_color=blue")
	result.MustFail(t, "UNKNOWN_FIELD")
	result.MustFailWithMessage(t, "schema type person")

	if result.Error == nil || result.Error.Details == nil {
		t.Fatalf("expected unknown field details in error, got: %#v", result.Error)
	}
	unknownFieldsRaw, ok := result.Error.Details["unknown_fields"].([]interface{})
	if !ok || len(unknownFieldsRaw) == 0 {
		t.Fatalf("expected unknown_fields in details, got: %#v", result.Error.Details)
	}
	if unknownFieldsRaw[0] != "favorite_color" {
		t.Fatalf("expected unknown field 'favorite_color', got: %#v", unknownFieldsRaw)
	}

	v.AssertFileNotExists("people/unknown-field-user.md")
}

func TestIntegration_SetFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Erin").MustSucceed(t)

	// email is a string field; JSON string "true" should stay a string.
	result := v.RunCLI("set", "people/erin", "--fields-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/erin.md", `email: "true"`)
}

func TestIntegration_SetBulkFieldsJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Bulk Erin One").MustSucceed(t)
	v.RunCLI("new", "person", "Bulk Erin Two").MustSucceed(t)

	result := v.RunCLIWithStdin(
		"people/bulk-erin-one\npeople/bulk-erin-two\n",
		"set",
		"--stdin",
		"--confirm",
		"--fields-json",
		`{"email":"true"}`,
	)
	result.MustSucceed(t)
	v.AssertFileContains("people/bulk-erin-one.md", `email: "true"`)
	v.AssertFileContains("people/bulk-erin-two.md", `email: "true"`)
}

func TestIntegration_UpsertFieldJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("upsert", "person", "Field Json User", "--field-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/field-json-user.md", `email: "true"`)
}

func TestIntegration_NewFieldJSONPreservesStringType(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("new", "person", "New Field Json User", "--field-json", `{"email":"true"}`)
	result.MustSucceed(t)
	v.AssertFileContains("people/new-field-json-user.md", `email: "true"`)
}

func TestIntegration_UpsertWithExplicitPath(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: note/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLI("upsert", "note", "Raven Friction", "--path", "note/raven-logo-brief", "--content", "# V1")
	result.MustSucceed(t)
	v.AssertFileExists("objects/note/raven-logo-brief.md")
	v.AssertFileContains("objects/note/raven-logo-brief.md", "# V1")

	result = v.RunCLI("upsert", "note", "Raven Friction", "--path", "note/raven-logo-brief", "--content", "# V2")
	result.MustSucceed(t)
	v.AssertFileContains("objects/note/raven-logo-brief.md", "# V2")
}

// TestIntegration_BulkOperationsPreview tests bulk operations with preview mode.
func TestIntegration_BulkOperationsPreview(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects
	v.RunCLI("new", "project", "Project A", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project B", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project C", "--field", "status=active").MustSucceed(t)

	// Preview bulk set without --confirm (should not apply changes) - uses == for comparison
	result := v.RunCLI("query", "type:project .status==active", "--apply", "set status=done")
	result.MustSucceed(t)

	// Files should still have active status since we didn't confirm
	v.AssertFileContains("projects/project-a.md", "status: active")

	// Now confirm the bulk operation
	result = v.RunCLI("query", "type:project .status==active", "--apply", "set status=done", "--confirm")
	result.MustSucceed(t)

	// Files should now have done status
	v.AssertFileContains("projects/project-a.md", "status: done")
	v.AssertFileContains("projects/project-b.md", "status: done")
	v.AssertFileContains("projects/project-c.md", "status: done")
}

// TestIntegration_BulkDelete tests bulk delete with confirmation.
func TestIntegration_BulkDelete(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects
	v.RunCLI("new", "project", "Project X", "--field", "status=done").MustSucceed(t)
	v.RunCLI("new", "project", "Project Y", "--field", "status=done").MustSucceed(t)

	// Bulk delete with confirmation - uses == for comparison
	result := v.RunCLI("query", "type:project .status==done", "--apply", "delete", "--confirm")
	result.MustSucceed(t)

	// Files should be deleted (moved to trash)
	v.AssertFileNotExists("projects/project-x.md")
	v.AssertFileNotExists("projects/project-y.md")
}

// TestIntegration_TraitBulkUpdate tests bulk update on trait query results.
func TestIntegration_TraitBulkUpdate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`).
		WithFile("tasks/task2.md", `---
type: page
---
# Task 2

- @priority(medium) Third task
`).
		Build()

	// Reindex to pick up the files
	v.RunCLI("reindex").MustSucceed(t)

	// Preview bulk update on low priority traits (should not apply)
	result := v.RunCLI("query", "trait:priority .value==low", "--apply", "update high")
	result.MustSucceed(t)

	// Files should still have low priority since we didn't confirm
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	// Now confirm the bulk operation
	result = v.RunCLI("query", "trait:priority .value==low", "--apply", "update high", "--confirm")
	result.MustSucceed(t)

	// Files should now have high priority
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(high) Second task")

	// The medium priority task should be unchanged
	v.AssertFileContains("tasks/task2.md", "@priority(medium) Third task")
}

// TestIntegration_TraitUpdateCommand tests the update command for trait IDs.
func TestIntegration_TraitUpdateCommand(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @priority(low) First task
- @priority(low) Second task
`).
		Build()

	// Reindex to pick up the files
	v.RunCLI("reindex").MustSucceed(t)

	// Single update by trait ID
	result := v.RunCLI("update", "tasks/task1.md:trait:0", "high")
	result.MustSucceed(t)
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	// Bulk update by stdin
	result = v.RunCLIWithStdin("tasks/task1.md:trait:1\n", "update", "--stdin", "medium", "--confirm")
	result.MustSucceed(t)
	v.AssertFileContains("tasks/task1.md", "@priority(medium) Second task")
}

func TestIntegration_TraitUpdateCommand_ResolvesRelativeDateKeyword(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @due(2026-01-01) Ship release
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("update", "tasks/task1.md:trait:0", "tomorrow")
	result.MustSucceed(t)

	tomorrow := time.Now().AddDate(0, 0, 1).Format("2006-01-02")
	v.AssertFileContains("tasks/task1.md", "@due("+tomorrow+")")
}

func TestIntegration_TraitUpdateRejectsInvalidEnumValue(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @priority(low) First task
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("update", "tasks/task1.md:trait:0", "critical")
	result.MustFailWithMessage(t, "invalid value for trait '@priority'")
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
}

func TestIntegration_TraitQueryApplyRejectsInvalidEnumValue(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @priority(low) First task
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("query", "trait:priority .value==low", "--apply", "update critical", "--confirm")
	result.MustFailWithMessage(t, "invalid value for trait '@priority'")
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
}

// TestIntegration_TraitBulkUpdateObjectCommandsRejected tests that object commands are rejected for trait queries.
func TestIntegration_TraitBulkUpdateObjectCommandsRejected(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("tasks/task1.md", `---
type: page
---
# Task 1

- @priority(low) First task
`).
		Build()

	// Reindex to pick up the files
	v.RunCLI("reindex").MustSucceed(t)

	// Try to use object commands on trait query - should fail
	result := v.RunCLI("query", "trait:priority", "--apply", "delete")
	result.MustFailWithMessage(t, "not supported for trait queries")

	result = v.RunCLI("query", "trait:priority", "--apply", "add some text")
	result.MustFailWithMessage(t, "not supported for trait queries")

	result = v.RunCLI("query", "trait:priority", "--apply", "move archive/")
	result.MustFailWithMessage(t, "not supported for trait queries")
}

// TestIntegration_CheckValidation tests the check command for validation.
func TestIntegration_CheckValidation(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("notes/orphan.md", `---
type: page
---
# Note with missing ref

See [[nonexistent/page]] for details.
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Run check - the check command has its own format (not the standard ok/data envelope)
	// Just verify the command runs and produces output
	result := v.RunCLI("check")

	// Check command output is structured differently - look at raw JSON
	if result.RawJSON == "" {
		t.Error("expected check to produce output")
	}

	// The raw JSON should contain issues for missing reference
	if !strings.Contains(result.RawJSON, "missing_reference") {
		t.Errorf("expected check output to include 'missing_reference' issue\nRaw: %s", result.RawJSON)
	}
}

func TestIntegration_CheckFixSubcommandAppliesShortRefFixes(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---`).
		WithFile("projects/roadmap.md", `---
type: project
title: Roadmap
owner: "[[freya]]"
---`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	preview := v.RunCLI("check", "fix")
	preview.MustSucceed(t)
	if got, ok := preview.Data["preview"].(bool); !ok || !got {
		t.Fatalf("expected preview=true, got %#v", preview.Data["preview"])
	}
	if got, ok := preview.Data["fixable_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 fixable issue, got %#v", preview.Data["fixable_issues"])
	}

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)
	if got, ok := apply.Data["preview"].(bool); !ok || got {
		t.Fatalf("expected preview=false after apply, got %#v", apply.Data["preview"])
	}
	if got, ok := apply.Data["fixed_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 fixed issue, got %#v", apply.Data["fixed_issues"])
	}

	v.AssertFileContains("projects/roadmap.md", "owner: \"[[people/freya]]\"")
}

// TestIntegration_CheckFixCanonicalPathMovesFiles verifies that check fix
// detects files outside the configured directory roots and migrates them via
// real file moves, with reference updates following the move.
func TestIntegration_CheckFixCanonicalPathMovesFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
`).
		WithRavenYAML(`directories:
  type: type/
  page: page/
`).
		WithFile("objects/person/john.md", `---
type: person
name: John
---
`).
		WithFile("page/notes/today.md", `---
type: page
---
Mentioned [[type/person/john]] today.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	preview := v.RunCLI("check", "fix")
	preview.MustSucceed(t)
	if got, ok := preview.Data["fixable_issues"].(float64); !ok || int(got) < 2 {
		t.Fatalf("expected at least 2 fixable issues (move + ref), got %#v", preview.Data["fixable_issues"])
	}

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)

	v.AssertFileExists("type/person/john.md")
	v.AssertFileNotExists("objects/person/john.md")

	v.AssertFileContains("page/notes/today.md", "[[person/john]]")
	v.AssertFileNotContains("page/notes/today.md", "[[type/person/john]]")
}

// TestIntegration_CheckFixCanonicalPathSkipsCollisions verifies that when a
// non_canonical_path move would collide with an existing file at the
// canonical destination, check fix skips that move and continues past it.
func TestIntegration_CheckFixCanonicalPathSkipsCollisions(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
`).
		WithRavenYAML(`directories:
  type: type/
  page: page/
`).
		WithFile("objects/person/john.md", `---
type: person
name: John (old)
---
`).
		WithFile("type/person/john.md", `---
type: person
name: John (new)
---
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	apply := v.RunCLI("check", "fix", "--confirm")
	apply.MustSucceed(t)

	if got, ok := apply.Data["skipped_issues"].(float64); !ok || int(got) < 1 {
		t.Fatalf("expected at least 1 skipped fix for collision, got %#v", apply.Data["skipped_issues"])
	}

	v.AssertFileExists("objects/person/john.md")
	v.AssertFileExists("type/person/john.md")
}

func TestIntegration_CheckCreateMissingSubcommandJSONConfirmRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/weekly.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Weekly
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// check create-missing may still exit non-zero due pre-existing validation issues;
	// validate side effects through file creation.
	_ = v.RunCLI("check", "create-missing", "--confirm")

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
}

// TestIntegration_CheckCreateMissingRespectsDirectoryRoots verifies that
// `check --create-missing` creates typed objects under configured directory roots.
func TestIntegration_CheckCreateMissingRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/kickoff.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Kickoff
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// check --create-missing is interactive and non-JSON; accept default "yes"
	// for "Certain (from typed fields)" prompts by sending an empty line.
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "check", "--create-missing")
	cmd.Stdin = strings.NewReader("\n")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Ensure the missing-reference creation flow ran.
	if !strings.Contains(outputStr, "Missing References") {
		t.Fatalf("expected check output to include missing reference prompt, got:\n%s", outputStr)
	}

	// Regression assertion: created file must be nested under objects root.
	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
	v.AssertFileContains("objects/meeting/all-hands.md", "type: meeting")
}

// TestIntegration_CheckCreateMissingUnknownTypeRespectsDirectoryRoots verifies
// the unknown-type interactive flow:
// 1) user provides a new type name
// 2) check creates the type in schema.yaml
// 3) missing page is created under configured objects root
func TestIntegration_CheckCreateMissingUnknownTypeRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  project:
    default_path: projects/
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/launch.md", `---
type: project
---
# Launch

See [[meeting/all-hands]] for notes.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// Interactive inputs:
	// - Type for meeting/all-hands: meeting
	// - Create new type meeting?: y
	// - Default path for meeting: meeting/
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "check", "--create-missing")
	cmd.Stdin = strings.NewReader("meeting\ny\nmeeting/\n")
	output, _ := cmd.CombinedOutput()
	outputStr := string(output)

	// Ensure the unknown-type flow ran.
	if !strings.Contains(outputStr, "Unknown type (please specify)") {
		t.Fatalf("expected check output to include unknown type prompt, got:\n%s", outputStr)
	}
	if !strings.Contains(outputStr, "Created type 'meeting' in schema.yaml") {
		t.Fatalf("expected check output to include type creation message, got:\n%s", outputStr)
	}

	// Regression assertion: created page must be nested under objects root.
	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
	v.AssertFileContains("objects/meeting/all-hands.md", "type: meeting")

	// Verify schema was updated with the new type/default_path.
	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meeting/")
}

// TestIntegration_CheckCreateMissingJSONConfirmRespectsDirectoryRoots verifies
// non-interactive create-missing in JSON mode (agent-style invocation).
func TestIntegration_CheckCreateMissingJSONConfirmRespectsDirectoryRoots(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  meeting:
    default_path: meeting/
  project:
    default_path: projects/
    fields:
      meeting:
        type: ref
        target: meeting
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		WithFile("projects/weekly.md", `---
type: project
meeting: "[[meeting/all-hands]]"
---
# Weekly
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	// Agent-style call: JSON mode + create-missing + confirm.
	binary := testutil.BuildCLI(t)
	cmd := exec.Command(binary, "--vault-path", v.Path, "--json", "check", "--create-missing", "--confirm")
	_, _ = cmd.CombinedOutput() // check may exit non-zero due validation issues; side effects are what we validate.

	v.AssertFileExists("objects/meeting/all-hands.md")
	v.AssertFileNotExists("meeting/all-hands.md")
}

// TestIntegration_ImportRespectsDirectoryRootsOnCreate verifies that imports
// create new objects through the canonical path resolution logic, including
// configured directory roots.
func TestIntegration_ImportRespectsDirectoryRootsOnCreate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
`).
		WithRavenYAML(`directories:
  type: objects/
`).
		Build()

	result := v.RunCLIWithStdin(`[{"name":"Freya"}]`, "import", "person")
	result.MustSucceed(t)

	v.AssertFileExists("objects/people/freya.md")
	v.AssertFileNotExists("people/freya.md")
	v.AssertFileNotExists("objects/objects/people/freya.md")
	v.AssertFileContains("objects/people/freya.md", "type: person")
	v.AssertFileContains("objects/people/freya.md", "name: Freya")
}

func TestIntegration_ImportUnknownFieldReturnsStructuredItemError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLIWithStdin(`[{"name":"Freya","favorite_color":"green"}]`, "import", "person")
	result.MustSucceed(t)

	if got, ok := result.Data["errors"].(float64); !ok || int(got) != 1 {
		t.Fatalf("expected errors=1, got: %#v", result.Data["errors"])
	}

	results := result.DataList("results")
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 import result item, got %d", len(results))
	}
	item, ok := results[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected import result object, got: %#v", results[0])
	}
	if item["action"] != "error" {
		t.Fatalf("expected import action=error, got: %#v", item["action"])
	}
	if item["code"] != "UNKNOWN_FIELD" {
		t.Fatalf("expected import error code UNKNOWN_FIELD, got: %#v", item["code"])
	}
	details, ok := item["details"].(map[string]interface{})
	if !ok || details == nil {
		t.Fatalf("expected structured details for import item error, got: %#v", item["details"])
	}
	unknownFields, ok := details["unknown_fields"].([]interface{})
	if !ok || len(unknownFields) == 0 {
		t.Fatalf("expected unknown_fields detail, got: %#v", details)
	}
	if unknownFields[0] != "favorite_color" {
		t.Fatalf("expected unknown field favorite_color, got: %#v", unknownFields)
	}

	v.AssertFileNotExists("people/freya.md")
}

func TestIntegration_AutoReindexDatabaseFailuresSurfaceStructuredWarnings(t *testing.T) {
	t.Parallel()

	breakIndex := func(t *testing.T, v *testutil.TestVault) {
		t.Helper()
		ravenDir := filepath.Join(v.Path, ".raven")
		if err := os.RemoveAll(ravenDir); err != nil {
			t.Fatalf("remove .raven: %v", err)
		}
		if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
			t.Fatalf("write .raven file: %v", err)
		}
	}

	assertIndexWarning := func(t *testing.T, result *testutil.CLIResult) {
		t.Helper()
		result.AssertHasWarning(t, "INDEX_UPDATE_FAILED")
		for _, warning := range result.Warnings {
			if warning.Code == "INDEX_UPDATE_FAILED" && strings.Contains(warning.Message, "failed to open index database") {
				return
			}
		}
		t.Fatalf("expected index warning mentioning database open failure, got warnings: %+v", result.Warnings)
	}

	tests := []struct {
		name   string
		run    func(v *testutil.TestVault) *testutil.CLIResult
		assert func(t *testing.T, v *testutil.TestVault)
	}{
		{
			name: "new",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("new", "person", "Freya")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/freya.md")
			},
		},
		{
			name: "upsert",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("upsert", "person", "Frigg", "--field", "email=frigg@example.com")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/frigg.md")
				v.AssertFileContains("people/frigg.md", "email: frigg@example.com")
			},
		},
		{
			name: "set",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("set", "people/alice", "email=alice@newdomain.com")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "email: alice@newdomain.com")
			},
		},
		{
			name: "add",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("add", "Follow up note", "--to", "people/alice")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "Follow up note")
			},
		},
		{
			name: "edit",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("edit", "people/alice", "Body", "Updated body", "--confirm")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileContains("people/alice.md", "Updated body")
			},
		},
		{
			name: "import",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLIWithStdin(`[{"name":"Thor"}]`, "import", "person")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("people/thor.md")
			},
		},
		{
			name: "template write",
			run: func(v *testutil.TestVault) *testutil.CLIResult {
				return v.RunCLI("template", "write", "meeting.md", "--content", "# {{title}}\n")
			},
			assert: func(t *testing.T, v *testutil.TestVault) {
				v.AssertFileExists("templates/meeting.md")
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("people/alice.md", `---
type: person
name: Alice
---

Body
`).
				Build()

			breakIndex(t, v)
			result := tc.run(v)
			result.MustSucceed(t)
			assertIndexWarning(t, result)
			tc.assert(t, v)
		})
	}
}

func TestIntegration_EditSurfacesAutoReindexParseWarnings(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
---

Body
`).
		Build()

	result := v.RunCLI("edit", "people/alice", "name: Alice", "name: [", "--confirm")
	result.MustSucceed(t)
	result.AssertHasWarning(t, "INDEX_UPDATE_FAILED")

	foundParseWarning := false
	for _, warning := range result.Warnings {
		if warning.Code == "INDEX_UPDATE_FAILED" && strings.Contains(warning.Message, "failed to parse file") {
			foundParseWarning = true
			break
		}
	}
	if !foundParseWarning {
		t.Fatalf("expected parse warning, got warnings: %+v", result.Warnings)
	}

	v.AssertFileContains("people/alice.md", "name: [")
}

// TestIntegration_NewPageRespectsPagesRoot verifies that creating a page type
// uses the configured pages root directory.
func TestIntegration_NewPageRespectsPagesRoot(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  type: objects/
  page: pages/
`).
		Build()

	result := v.RunCLI("new", "page", "Quick Note")
	result.MustSucceed(t)

	v.AssertFileExists("pages/quick-note.md")
	v.AssertFileNotExists("quick-note.md")
	v.AssertFileNotExists("objects/quick-note.md")
	v.AssertFileContains("pages/quick-note.md", "type: page")
}

func TestIntegration_InvalidRavenYAMLFailsCommands(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  type: [
`).
		Build()

	result := v.RunCLI("new", "page", "Broken Config Note")
	result.MustFail(t, "CONFIG_INVALID")
	result.MustFailWithMessage(t, "failed to load raven.yaml")

	v.AssertFileNotExists("broken-config-note.md")
}

func TestIntegration_ReadCommandsClassifyInvalidRavenYAMLAsConfigError(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "anything"}},
		{name: "backlinks", args: []string{"backlinks", "anything"}},
		{name: "outlinks", args: []string{"outlinks", "anything"}},
		{name: "resolve", args: []string{"resolve", "anything"}},
		{name: "read", args: []string{"read", "anything"}},
		{name: "open", args: []string{"open", "anything"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.MinimalSchema()).
				WithRavenYAML(`directories:
  type: [
`).
				Build()

			result := v.RunCLI(tc.args...)
			result.MustFail(t, "CONFIG_INVALID")
			result.MustFailWithMessage(t, "failed to load raven.yaml")
		})
	}
}

func TestIntegration_ReadCommandsClassifyDatabaseBootstrapFailures(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
	}{
		{name: "search", args: []string{"search", "anything"}},
		{name: "backlinks", args: []string{"backlinks", "anything"}},
		{name: "outlinks", args: []string{"outlinks", "anything"}},
		{name: "resolve", args: []string{"resolve", "anything"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(testutil.MinimalSchema()).
				Build()

			ravenDir := filepath.Join(v.Path, ".raven")
			if err := os.RemoveAll(ravenDir); err != nil {
				t.Fatalf("remove .raven directory: %v", err)
			}
			if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
				t.Fatalf("write .raven file: %v", err)
			}

			result := v.RunCLI(tc.args...)
			result.MustFail(t, "DATABASE_ERROR")
			result.MustFailWithMessage(t, "rvn reindex")
		})
	}
}

// TestIntegration_Search tests full-text search.
func TestIntegration_Search(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/meeting.md", `---
type: page
---
# Team Meeting Notes

Discussed the quarterly roadmap and budget allocation.
`).
		WithFile("notes/todo.md", `---
type: page
---
# Todo List

- Review quarterly report
- Prepare presentation
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Search for quarterly
	result := v.RunCLI("search", "quarterly")
	result.MustSucceed(t)

	// Should find both files (may return more than 2 results because section
	// objects are also indexed — e.g. "# Team Meeting Notes" produces both a
	// page-level and a section-level FTS entry).
	results := result.DataList("results")
	if len(results) < 2 {
		t.Errorf("expected at least 2 search results, got %d", len(results))
	}
}

// TestIntegration_DailyNote tests daily note creation and management.
func TestIntegration_DailyNote(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`version: 1
daily:
  directory: daily/
  template: |
    # Daily Note
    
    ## Tasks
`).
		Build()

	// The daily command may output human-readable text or JSON
	// Just verify that running it creates the daily directory
	_ = v.RunCLI("daily")

	// Verify the daily directory exists
	v.AssertDirExists("daily")
}

// TestIntegration_Add tests adding content to files.
func TestIntegration_Add(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("inbox.md", `---
type: page
---
# Inbox
`).
		Build()

	// Add content to inbox
	result := v.RunCLI("add", "New task for today", "--to", "inbox.md")
	result.MustSucceed(t)

	v.AssertFileContains("inbox.md", "New task for today")
}

func TestIntegration_AddToSectionBySlug(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item

### Other
- Keep this below
`).
		Build()

	result := v.RunCLI("add", "New bug item", "--to", "project.md", "--heading", "bugs-fixes")
	result.MustSucceed(t)

	content, err := os.ReadFile(filepath.Join(v.Path, "project.md"))
	if err != nil {
		t.Fatalf("read project.md: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "New bug item") {
		t.Fatalf("expected new section content in project.md, got:\n%s", text)
	}
	if strings.Index(text, "New bug item") > strings.Index(text, "### Other") {
		t.Fatalf("expected section append before next heading, got:\n%s", text)
	}
}

func TestIntegration_AddToSectionByHeadingText(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item
`).
		Build()

	result := v.RunCLI("add", "Another bug item", "--to", "project.md", "--heading", "### Bugs / Fixes")
	result.MustSucceed(t)
	v.AssertFileContains("project.md", "Another bug item")
}

func TestIntegration_AddToSectionReportsInsertedLine(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item

### Other
- Keep this below
`).
		Build()

	result := v.RunCLI("add", "Another bug item", "--to", "project.md", "--heading", "### Bugs / Fixes")
	result.MustSucceed(t)

	lineValue, ok := result.Data["line"].(float64)
	if !ok {
		t.Fatalf("expected numeric line in result data, got %#v", result.Data["line"])
	}
	if int(lineValue) != 8 {
		t.Fatalf("line = %v, want 8", lineValue)
	}
}

func TestIntegration_AddReportsStableHeadingErrorCodes(t *testing.T) {
	t.Parallel()

	t.Run("missing heading returns ref not found", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("project.md", `---
type: page
---
# Project

### Existing Heading
- Existing item
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "project.md", "--heading", "### Missing Heading")
		result.MustFail(t, "REF_NOT_FOUND")
	})

	t.Run("ambiguous heading text returns ref ambiguous", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("project.md", `---
type: page
---
# Project

### Team Notes
First section

### Team Notes
Second section
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "project.md", "--heading", "### Team Notes")
		result.MustFail(t, "REF_AMBIGUOUS")
	})

	t.Run("heading parse failure returns invalid input", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("broken.md", `---
type: page
meta:
  nested: true
---
# Broken
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "broken.md", "--heading", "### Broken")
		result.MustFail(t, "INVALID_INPUT")
	})
}

func TestIntegration_ResolveAndAddPreferDynamicTodayOverSectionShortName(t *testing.T) {
	t.Parallel()
	today := time.Now().Format("2006-01-02")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-03-16.md", `# Archive

# today
Old note
`).
		WithFile("daily/"+today+".md", `# Today
Current note
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	resolveResult := v.RunCLI("resolve", "today")
	resolveResult.MustSucceed(t)
	if resolveResult.DataString("object_id") != "daily/"+today {
		t.Fatalf("resolve object_id = %q, want %q", resolveResult.DataString("object_id"), "daily/"+today)
	}

	addResult := v.RunCLI("add", "New task for today", "--to", "today")
	addResult.MustSucceed(t)
	currentDaily := v.ReadFile("daily/" + today + ".md")
	if !strings.Contains(currentDaily, "New task for today") {
		t.Fatalf("expected current daily note to contain capture, got:\n%s", currentDaily)
	}
	archivedDaily := v.ReadFile("daily/2026-03-16.md")
	if strings.Contains(archivedDaily, "New task for today") {
		t.Fatalf("expected archived daily note to remain unchanged, got:\n%s", archivedDaily)
	}
}

// TestIntegration_DuplicateObjectError tests that creating a duplicate object fails.
func TestIntegration_DuplicateObjectError(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create a person
	v.RunCLI("new", "person", "Alice").MustSucceed(t)

	// Try to create the same person again
	result := v.RunCLI("new", "person", "Alice")
	result.MustFail(t, "FILE_EXISTS")
}

// TestIntegration_ReadObject tests reading object content.
func TestIntegration_ReadObject(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/bob.md", `---
type: person
name: Bob
email: bob@example.com
---
# Bob

Bob is a software engineer.
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Read the object
	result := v.RunCLI("read", "people/bob")
	result.MustSucceed(t)

	// Verify we got the content back
	content := result.DataString("content")
	if content == "" {
		t.Errorf("expected content in read result, got empty string")
	}
}

func TestIntegration_ReadSupportsDynamicDateReferences(t *testing.T) {
	t.Parallel()

	today := time.Now().Format("2006-01-02")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("today.md", "# Literal Today\n").
		WithFile("daily/"+today+".md", `---
type: page
---
# Daily Today
First line
Second line
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("read", "today")
	result.MustSucceed(t)
	if got := result.DataString("path"); got != "daily/"+today+".md" {
		t.Fatalf("path = %q, want %q", got, "daily/"+today+".md")
	}
	content := result.DataString("content")
	if !strings.Contains(content, "# Daily Today") {
		t.Fatalf("expected dynamic daily content, got:\n%s", content)
	}
	if strings.Contains(content, "# Literal Today") {
		t.Fatalf("expected read today to prefer the daily note, got:\n%s", content)
	}

	raw := v.RunCLI("read", "today", "--raw", "--start-line", "4", "--end-line", "5")
	raw.MustSucceed(t)
	if got := raw.DataString("path"); got != "daily/"+today+".md" {
		t.Fatalf("raw path = %q, want %q", got, "daily/"+today+".md")
	}
	rawContent := raw.DataString("content")
	if !strings.Contains(rawContent, "# Daily Today") {
		t.Fatalf("expected ranged read to use dynamic daily target, got:\n%s", rawContent)
	}
}

func TestIntegration_ISODateRefsAreAmbiguousOnCollision(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("2025-02-01.md", `---
type: page
---
# Literal ISO Note
`).
		WithFile("daily/2025-02-01.md", `---
type: page
---
# Daily ISO Note
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	resolve := v.RunCLI("resolve", "2025-02-01")
	resolve.MustSucceed(t)
	if resolve.Data["resolved"] != false {
		t.Fatalf("expected resolved=false for ambiguous ISO date, got %#v", resolve.Data["resolved"])
	}
	if resolve.Data["ambiguous"] != true {
		t.Fatalf("expected ambiguous=true for ambiguous ISO date, got %#v", resolve.Data["ambiguous"])
	}
	matches := resolve.DataList("matches")
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %#v", matches)
	}
	matchIDs := make(map[string]bool, len(matches))
	for _, raw := range matches {
		match, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected match object, got %#v", raw)
		}
		id, _ := match["object_id"].(string)
		matchIDs[id] = true
	}
	if !matchIDs["2025-02-01"] || !matchIDs["daily/2025-02-01"] {
		t.Fatalf("expected ISO collision matches for literal and daily notes, got %#v", matches)
	}

	read := v.RunCLI("read", "2025-02-01")
	read.MustFail(t, "REF_AMBIGUOUS")

	query := v.RunCLI("query", "type:page refs([[2025-02-01]])")
	query.MustFail(t, "QUERY_INVALID")
	query.MustFailWithMessage(t, "ambiguous reference '2025-02-01'")
}

func TestIntegration_ReadWithoutArgSuggestsUsage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("read")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn read <reference>")
}

func TestIntegration_OpenWithoutArgSuggestsUsage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("open")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn open <reference>")
}

// TestIntegration_Resolve tests the resolve command.
func TestIntegration_Resolve(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---
# Freya
`).
		WithFile("people/thor.md", `---
type: person
name: Thor
---
# Thor
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	t.Run("resolve by literal path", func(t *testing.T) {
		result := v.RunCLI("resolve", "people/freya")
		result.MustSucceed(t)

		if result.DataString("object_id") != "people/freya" {
			t.Errorf("expected object_id 'people/freya', got %q", result.DataString("object_id"))
		}
		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("type") != "person" {
			t.Errorf("expected type 'person', got %q", result.DataString("type"))
		}
		if result.DataString("match_source") != "literal_path" {
			t.Errorf("expected match_source 'literal_path', got %q", result.DataString("match_source"))
		}
	})

	t.Run("resolve by short name", func(t *testing.T) {
		result := v.RunCLI("resolve", "thor")
		result.MustSucceed(t)

		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("object_id") != "people/thor" {
			t.Errorf("expected object_id 'people/thor', got %q", result.DataString("object_id"))
		}
	})

	t.Run("resolve not found", func(t *testing.T) {
		result := v.RunCLI("resolve", "nonexistent")
		result.MustSucceed(t)

		if result.Data["resolved"] != false {
			t.Errorf("expected resolved=false for not-found ref")
		}
	})

	t.Run("resolve with .md extension", func(t *testing.T) {
		result := v.RunCLI("resolve", "people/freya.md")
		result.MustSucceed(t)

		if result.Data["resolved"] != true {
			t.Errorf("expected resolved=true")
		}
		if result.DataString("object_id") != "people/freya" {
			t.Errorf("expected object_id 'people/freya', got %q", result.DataString("object_id"))
		}
	})
}

// TestIntegration_SchemaTemplateLifecycle tests schema template lifecycle commands.
func TestIntegration_SchemaTemplateLifecycle(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	t.Run("schema template set/get/remove", func(t *testing.T) {
		v.WriteFile("templates/person.md", "# Person Profile\n")

		result := v.RunCLI("schema", "template", "set", "person_profile", "--file", "templates/person.md")
		result.MustSucceed(t)

		result = v.RunCLI("schema", "template", "get", "person_profile")
		result.MustSucceed(t)
		if result.DataString("id") != "person_profile" {
			t.Errorf("expected id=person_profile, got %q", result.DataString("id"))
		}
		if result.DataString("file") != "templates/person.md" {
			t.Errorf("expected file 'templates/person.md', got %q", result.DataString("file"))
		}

		v.RunCLI("schema", "template", "bind", "person_profile", "--type", "person").MustSucceed(t)
		v.RunCLI("schema", "template", "default", "person_profile", "--type", "person").MustSucceed(t)
		v.RunCLI("new", "person", "Alice").MustSucceed(t)
		v.AssertFileContains("people/alice.md", "# Person Profile")

		result = v.RunCLI("schema", "template", "unbind", "person_profile", "--type", "person")
		result.MustFailWithMessage(t, "--clear-default")

		result = v.RunCLI("schema", "template", "unbind", "person_profile", "--type", "person", "--clear-default")
		result.MustSucceed(t)
		if result.Data["removed"] != true {
			t.Errorf("expected removed=true")
		}

		result = v.RunCLI("schema", "template", "remove", "person_profile")
		result.MustSucceed(t)
		if result.Data["removed"] != true {
			t.Errorf("expected schema template remove=true")
		}
	})

	t.Run("daily lifecycle via date type templates", func(t *testing.T) {
		v.WriteFile("templates/daily.md", "# {{weekday}}, {{date}}\n\n## Notes\n")
		v.WriteFile("templates/daily-brief.md", "# {{date}}\n\n## Brief\n")

		result := v.RunCLI("schema", "template", "set", "daily_default", "--file", "templates/daily.md")
		result.MustSucceed(t)
		if result.DataString("file") != "templates/daily.md" {
			t.Errorf("expected daily file binding to templates/daily.md, got %q", result.DataString("file"))
		}
		v.RunCLI("schema", "template", "set", "daily_brief", "--file", "templates/daily-brief.md").MustSucceed(t)

		result = v.RunCLI("schema", "template", "bind", "daily_default", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("bind core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "bind", "daily_brief", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("bind core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "default", "daily_default", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("default core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "list", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("list core = %q, want %q", got, "date")
		}

		v.RunCLI("daily", "2026-02-03").MustSucceed(t)
		v.AssertFileContains("daily/2026-02-03.md", "## Notes")
		v.RunCLI("daily", "2026-02-05", "--template", "daily_brief").MustSucceed(t)
		v.AssertFileContains("daily/2026-02-05.md", "## Brief")

		result = v.RunCLI("schema", "template", "default", "--core", "date", "--clear")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("clear default core = %q, want %q", got, "date")
		}
		result = v.RunCLI("schema", "template", "unbind", "daily_brief", "--core", "date")
		result.MustSucceed(t)
		if got := result.DataString("core"); got != "date" {
			t.Fatalf("unbind core = %q, want %q", got, "date")
		}
		v.RunCLI("daily", "2026-02-04").MustSucceed(t)
		v.AssertFileNotContains("daily/2026-02-04.md", "## Notes")
	})
}

func TestIntegration_ReclassifyRejectsMalformedFieldFlags(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		field string
	}{
		{name: "missing equals", field: "author"},
		{name: "empty key", field: "=Tolkien"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			v := testutil.NewTestVault(t).
				WithSchema(`version: 2
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
`).
				WithFile("notes/my-note.md", `---
type: note
title: My Note
---

Body
`).
				Build()

			result := v.RunCLI("reclassify", "notes/my-note", "book", "--field", tc.field, "--no-move", "--force")
			result.MustFail(t, "INVALID_INPUT")
			result.MustFailWithMessage(t, "expected key=value")

			v.AssertFileExists("notes/my-note.md")
			v.AssertFileNotExists("books/my-note.md")
			v.AssertFileContains("notes/my-note.md", "type: note")
			v.AssertFileNotContains("notes/my-note.md", "type: book")
		})
	}
}

// TestIntegration_BacklinksOutlinksDynamicDates tests that backlinks and outlinks
// resolve dynamic date keywords like "today" and "yesterday".
func TestIntegration_BacklinksOutlinksDynamicDates(t *testing.T) {
	t.Parallel()
	today := time.Now().Format("2006-01-02")

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("directories:\n  daily: daily/\n").
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice
`).
		WithFile("daily/"+today+".md", `---
type: page
---
# Daily Note

Met with [[people/alice]] today.
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	t.Run("backlinks with dynamic date today", func(t *testing.T) {
		// "today" should resolve to daily/<today> and alice should have a backlink from it
		result := v.RunCLI("backlinks", "alice")
		result.MustSucceed(t)
		result.AssertResultCount(t, "items", 1)

		// Now test that "today" resolves as a target for backlinks
		result = v.RunCLI("backlinks", "today")
		result.MustSucceed(t)

		if result.DataString("target") != "daily/"+today {
			t.Errorf("expected target 'daily/%s', got %q", today, result.DataString("target"))
		}
	})

	t.Run("outlinks with dynamic date today", func(t *testing.T) {
		result := v.RunCLI("outlinks", "today")
		result.MustSucceed(t)

		if result.DataString("source") != "daily/"+today {
			t.Errorf("expected source 'daily/%s', got %q", today, result.DataString("source"))
		}
		result.AssertResultCount(t, "items", 1)
	})
}
