//go:build integration

package cli_test

import (
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_ObjectLifecycle tests creating, querying, updating, and deleting objects.
func TestIntegration_ObjectLifecycle(t *testing.T) {
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
	result = v.RunCLI("query", "object:person")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)

	// Update the person's email (set uses positional field=value args)
	result = v.RunCLI("set", "people/alice", "email=alice@newdomain.com")
	result.MustSucceed(t)
	v.AssertFileContains("people/alice.md", "email: alice@newdomain.com")

	// Delete the person
	result = v.RunCLI("delete", "people/alice", "--force")
	result.MustSucceed(t)
	v.AssertFileNotExists("people/alice.md")
}

// TestIntegration_QueryByField tests querying objects by field values.
func TestIntegration_QueryByField(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects with different statuses
	v.RunCLI("new", "project", "Project Alpha", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project Beta", "--field", "status=paused").MustSucceed(t)
	v.RunCLI("new", "project", "Project Gamma", "--field", "status=active").MustSucceed(t)

	// Query for active projects - uses == for equality
	result := v.RunCLI("query", "object:project .status==active")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 2)

	// Query for paused projects
	result = v.RunCLI("query", "object:project .status==paused")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

// TestIntegration_ReferencesAndBacklinks tests reference resolution and backlinks.
func TestIntegration_ReferencesAndBacklinks(t *testing.T) {
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
	result := v.RunCLI("query", "object:project refs([[people/alice]])")
	result.MustSucceed(t)
	result.AssertResultCount(t, "items", 1)
}

// TestIntegration_TraitQueries tests trait queries with various predicates.
func TestIntegration_TraitQueries(t *testing.T) {
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

// TestIntegration_SchemaValidationErrors tests that schema validation errors are properly reported.
func TestIntegration_SchemaValidationErrors(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Try to create a person without providing the required fields via title
	// Create the file manually without a required field and verify check finds the issue
	v.RunCLI("new", "person", "TestPerson").MustSucceed(t)

	// Verify that trying to set an invalid enum value fails
	result := v.RunCLI("set", "people/testperson", "status=invalid_value")
	// This should succeed but with a warning about unknown field
	// because status is not a valid field for person type
	result.MustSucceed(t)
	result.AssertHasWarning(t, "UNKNOWN_FIELD")
}

// TestIntegration_BulkOperationsPreview tests bulk operations with preview mode.
func TestIntegration_BulkOperationsPreview(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects
	v.RunCLI("new", "project", "Project A", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project B", "--field", "status=active").MustSucceed(t)
	v.RunCLI("new", "project", "Project C", "--field", "status=active").MustSucceed(t)

	// Preview bulk set without --confirm (should not apply changes) - uses == for comparison
	result := v.RunCLI("query", "object:project .status==active", "--apply", "set status=done")
	result.MustSucceed(t)

	// Files should still have active status since we didn't confirm
	v.AssertFileContains("projects/project-a.md", "status: active")

	// Now confirm the bulk operation
	result = v.RunCLI("query", "object:project .status==active", "--apply", "set status=done", "--confirm")
	result.MustSucceed(t)

	// Files should now have done status
	v.AssertFileContains("projects/project-a.md", "status: done")
	v.AssertFileContains("projects/project-b.md", "status: done")
	v.AssertFileContains("projects/project-c.md", "status: done")
}

// TestIntegration_BulkDelete tests bulk delete with confirmation.
func TestIntegration_BulkDelete(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	// Create multiple projects
	v.RunCLI("new", "project", "Project X", "--field", "status=done").MustSucceed(t)
	v.RunCLI("new", "project", "Project Y", "--field", "status=done").MustSucceed(t)

	// Bulk delete with confirmation - uses == for comparison
	result := v.RunCLI("query", "object:project .status==done", "--apply", "delete", "--confirm")
	result.MustSucceed(t)

	// Files should be deleted (moved to trash)
	v.AssertFileNotExists("projects/project-x.md")
	v.AssertFileNotExists("projects/project-y.md")
}

// TestIntegration_TraitBulkUpdate tests bulk update on trait query results.
func TestIntegration_TraitBulkUpdate(t *testing.T) {
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
	result := v.RunCLI("query", "trait:priority .value==low", "--apply", "update value=high")
	result.MustSucceed(t)

	// Files should still have low priority since we didn't confirm
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	// Now confirm the bulk operation
	result = v.RunCLI("query", "trait:priority .value==low", "--apply", "update value=high", "--confirm")
	result.MustSucceed(t)

	// Files should now have high priority
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(high) Second task")

	// The medium priority task should be unchanged
	v.AssertFileContains("tasks/task2.md", "@priority(medium) Third task")
}

// TestIntegration_TraitUpdateCommand tests the update command for trait IDs.
func TestIntegration_TraitUpdateCommand(t *testing.T) {
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
	result := v.RunCLI("update", "tasks/task1.md:trait:0", "value=high")
	result.MustSucceed(t)
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	// Bulk update by stdin
	result = v.RunCLIWithStdin("tasks/task1.md:trait:1\n", "update", "--stdin", "value=done", "--confirm")
	result.MustSucceed(t)
	v.AssertFileContains("tasks/task1.md", "@priority(done) Second task")
}

// TestIntegration_TraitBulkUpdateObjectCommandsRejected tests that object commands are rejected for trait queries.
func TestIntegration_TraitBulkUpdateObjectCommandsRejected(t *testing.T) {
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

// TestIntegration_CheckCreateMissingRespectsDirectoryRoots verifies that
// `check --create-missing` creates typed objects under configured directory roots.
func TestIntegration_CheckCreateMissingRespectsDirectoryRoots(t *testing.T) {
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
  object: objects/
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

	// Ensure the missing-reference workflow ran.
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
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  project:
    default_path: projects/
`).
		WithRavenYAML(`directories:
  object: objects/
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
  object: objects/
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
  object: objects/
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

// TestIntegration_NewPageRespectsPagesRoot verifies that creating a page type
// uses the configured pages root directory.
func TestIntegration_NewPageRespectsPagesRoot(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  object: objects/
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
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  object: [
`).
		Build()

	result := v.RunCLI("new", "page", "Broken Config Note")
	result.MustFail(t, "CONFIG_INVALID")
	result.MustFailWithMessage(t, "failed to load raven.yaml")

	v.AssertFileNotExists("broken-config-note.md")
}

func TestIntegration_WorkflowManagementCommands(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	// Add a file-backed workflow.
	v.WriteFile("workflows/inline-brief.yaml", `description: File workflow
steps:
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Return JSON: {"outputs":{"markdown":"ok"}}
`)
	result := v.RunCLI("workflow", "add", "inline-brief", "--file", "workflows/inline-brief.yaml")
	result.MustSucceed(t)

	// Files outside directories.workflow are rejected.
	v.WriteFile("automation/outside.yaml", `description: Outside workflow
steps:
  - id: compose
    type: agent
    prompt: test
`)
	v.RunCLI("workflow", "add", "outside", "--file", "automation/outside.yaml").MustFail(t, "INVALID_INPUT")

	// Validate all workflows.
	result = v.RunCLI("workflow", "validate")
	result.MustSucceed(t)
	if valid, _ := result.Data["valid"].(bool); !valid {
		t.Fatalf("expected workflow validate to return valid=true, got: %v", result.RawJSON)
	}

	// Add a file-backed workflow.
	v.WriteFile("workflows/file-brief.yaml", `description: File workflow
steps:
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Return JSON: {"outputs":{"markdown":"ok"}}
`)
	result = v.RunCLI("workflow", "add", "file-brief", "--file", "workflows/file-brief.yaml")
	result.MustSucceed(t)

	// Bare filename should resolve under directories.workflow.
	v.WriteFile("workflows/bare-brief.yaml", `description: Bare file workflow
steps:
  - id: compose
    type: agent
    outputs:
      markdown:
        type: markdown
        required: true
    prompt: |
      Return JSON: {"outputs":{"markdown":"ok"}}
`)
	result = v.RunCLI("workflow", "add", "bare-brief", "--file", "bare-brief.yaml")
	result.MustSucceed(t)
	v.AssertFileContains("raven.yaml", "file: workflows/bare-brief.yaml")

	// Confirm list includes all registered workflows.
	result = v.RunCLI("workflow", "list")
	result.MustSucceed(t)
	items := result.DataList("workflows")
	if len(items) != 3 {
		t.Fatalf("expected 3 workflows, got %d\nRaw: %s", len(items), result.RawJSON)
	}

	// Remove one workflow and ensure it is gone.
	v.RunCLI("workflow", "remove", "inline-brief").MustSucceed(t)
	result = v.RunCLI("workflow", "show", "inline-brief")
	result.MustFail(t, "QUERY_NOT_FOUND")
}

func TestIntegration_WorkflowScaffold(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	// Existing file should block scaffold without --force.
	v.WriteFile("workflows/custom.yaml", "description: existing\nsteps:\n  - id: compose\n    type: agent\n    prompt: existing\n")
	result := v.RunCLI("workflow", "scaffold", "starter", "--file", "workflows/custom.yaml")
	result.MustFail(t, "FILE_EXISTS")

	// --force allows overwrite and registration in raven.yaml.
	result = v.RunCLI("workflow", "scaffold", "starter", "--file", "workflows/custom.yaml", "--force")
	result.MustSucceed(t)
	v.AssertFileExists("workflows/custom.yaml")
	v.AssertFileContains("workflows/custom.yaml", "type: tool")
	v.AssertFileContains("workflows/custom.yaml", "type: agent")

	// Bare --file is resolved under directories.workflow.
	result = v.RunCLI("workflow", "scaffold", "starter-bare", "--file", "starter-bare.yaml")
	result.MustSucceed(t)
	v.AssertFileExists("workflows/starter-bare.yaml")
	v.AssertFileContains("raven.yaml", "file: workflows/starter-bare.yaml")

	// Scaffolded workflow should be valid and runnable via workflow commands.
	v.RunCLI("workflow", "validate", "starter").MustSucceed(t)
	v.RunCLI("workflow", "show", "starter").MustSucceed(t)

	// Enforce configured directories.workflow for custom file paths.
	v.RunCLI("workflow", "scaffold", "bad", "--file", "automation/outside.yaml").MustFail(t, "INVALID_INPUT")
}

func TestIntegration_WorkflowScaffold_CustomWorkflowDirectory(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`directories:
  workflow: automation/workflows/
`).
		Build()

	result := v.RunCLI("workflow", "scaffold", "starter")
	result.MustSucceed(t)

	v.AssertFileExists("automation/workflows/starter.yaml")
	v.RunCLI("workflow", "show", "starter").MustSucceed(t)
}

func TestIntegration_WorkflowValidateReportsInvalidDefinitions(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`workflows:
  broken:
    description: Broken workflow
    steps:
      - type: tool
        tool: raven_query
`).
		Build()

	result := v.RunCLI("workflow", "validate")
	result.MustFail(t, "WORKFLOW_INVALID")
	if result.Error == nil || !strings.Contains(result.Error.Message, "invalid") {
		t.Fatalf("expected workflow validation failure message, got: %s", result.RawJSON)
	}
}

// TestIntegration_Search tests full-text search.
func TestIntegration_Search(t *testing.T) {
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

// TestIntegration_DuplicateObjectError tests that creating a duplicate object fails.
func TestIntegration_DuplicateObjectError(t *testing.T) {
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

func TestIntegration_ReadWithoutArgSuggestsUsage(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("read")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn read <reference>")
}

func TestIntegration_OpenWithoutArgSuggestsUsage(t *testing.T) {
	v := testutil.NewTestVault(t).Build()

	result := v.RunCLI("open")
	result.MustFail(t, "MISSING_ARGUMENT")
	result.MustFailWithMessage(t, "rvn open <reference>")
}

// TestIntegration_Resolve tests the resolve command.
func TestIntegration_Resolve(t *testing.T) {
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

// TestIntegration_TemplateLifecycle tests top-level template lifecycle commands.
func TestIntegration_TemplateLifecycle(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	t.Run("get with no template", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "person")
		result.MustSucceed(t)

		if result.Data["configured"] != false {
			t.Errorf("expected configured=false, got %#v", result.Data["configured"])
		}
	})

	t.Run("set file template", func(t *testing.T) {
		v.WriteFile("templates/person.md", "# {{title}}\n\nEmail: {{field.email}}")
		result := v.RunCLI("template", "set", "type", "person", "--file", "templates/person.md")
		result.MustSucceed(t)

		if result.DataString("file") != "templates/person.md" {
			t.Errorf("expected file 'templates/person.md', got %q", result.DataString("file"))
		}
	})

	t.Run("get after set", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "person")
		result.MustSucceed(t)

		if result.Data["configured"] != true {
			t.Errorf("expected configured=true")
		}
		if result.DataString("content") == "" {
			t.Errorf("expected non-empty template content")
		}
	})

	t.Run("render template", func(t *testing.T) {
		result := v.RunCLI("template", "render", "type", "person", "--title", "Alice")
		result.MustSucceed(t)

		rendered := result.DataString("rendered")
		if !strings.Contains(rendered, "# Alice") {
			t.Errorf("expected rendered to contain '# Alice', got %q", rendered)
		}
	})

	t.Run("get file-based template resolves content", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "person")
		result.MustSucceed(t)

		if result.DataString("content") == "" {
			t.Errorf("expected non-empty content for file-based template")
		}
	})

	t.Run("remove template", func(t *testing.T) {
		result := v.RunCLI("template", "remove", "type", "person")
		result.MustSucceed(t)

		if result.Data["removed"] != true {
			t.Errorf("expected removed=true")
		}
	})

	t.Run("get after remove", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "person")
		result.MustSucceed(t)

		if result.Data["configured"] != false {
			t.Errorf("expected configured=false after remove, got %#v", result.Data["configured"])
		}
	})

	t.Run("error on built-in type", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "page")
		if result.OK {
			t.Errorf("expected error for built-in type")
		}
	})

	t.Run("error on unknown type", func(t *testing.T) {
		result := v.RunCLI("template", "get", "type", "nonexistent")
		if result.OK {
			t.Errorf("expected error for unknown type")
		}
	})

	t.Run("daily lifecycle", func(t *testing.T) {
		v.WriteFile("templates/daily.md", "# {{weekday}}, {{date}}\n\n## Notes\n")

		result := v.RunCLI("template", "set", "daily", "--file", "templates/daily.md")
		result.MustSucceed(t)
		if result.DataString("file") != "templates/daily.md" {
			t.Errorf("expected daily file binding to templates/daily.md, got %q", result.DataString("file"))
		}

		result = v.RunCLI("template", "get", "daily")
		result.MustSucceed(t)
		if result.Data["configured"] != true {
			t.Errorf("expected daily configured=true")
		}

		result = v.RunCLI("template", "write", "daily", "--content", "# {{weekday}}, {{date}}\n\n## Morning\n")
		result.MustSucceed(t)
		v.AssertFileContains("templates/daily.md", "## Morning")

		result = v.RunCLI("template", "render", "daily", "--date", "tomorrow")
		result.MustSucceed(t)
		if rendered := result.DataString("rendered"); !strings.Contains(rendered, "## Morning") {
			t.Errorf("expected rendered daily template to include updated content, got %q", rendered)
		}

		result = v.RunCLI("template", "remove", "daily", "--delete-file")
		result.MustSucceed(t)
		if result.Data["removed"] != true {
			t.Errorf("expected removed=true for daily template")
		}
		v.AssertFileNotExists("templates/daily.md")
	})
}

// TestIntegration_BacklinksOutlinksDynamicDates tests that backlinks and outlinks
// resolve dynamic date keywords like "today" and "yesterday".
func TestIntegration_BacklinksOutlinksDynamicDates(t *testing.T) {
	today := time.Now().Format("2006-01-02")

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("daily_directory: daily\n").
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
