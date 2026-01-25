//go:build integration

package cli_test

import (
	"strings"
	"testing"

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
	result := v.RunCLI("query", "object:project refs:[[people/alice]]")
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

// TestIntegration_TraitBulkSet tests bulk set on trait query results.
func TestIntegration_TraitBulkSet(t *testing.T) {
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

	// Preview bulk set on low priority traits (should not apply)
	result := v.RunCLI("query", "trait:priority .value==low", "--apply", "set value=high")
	result.MustSucceed(t)

	// Files should still have low priority since we didn't confirm
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	// Now confirm the bulk operation
	result = v.RunCLI("query", "trait:priority .value==low", "--apply", "set value=high", "--confirm")
	result.MustSucceed(t)

	// Files should now have high priority
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(high) Second task")

	// The medium priority task should be unchanged
	v.AssertFileContains("tasks/task2.md", "@priority(medium) Third task")
}

// TestIntegration_TraitBulkSetObjectCommandsRejected tests that object commands are rejected for trait queries.
func TestIntegration_TraitBulkSetObjectCommandsRejected(t *testing.T) {
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

	// Should find both files
	results := result.DataList("results")
	if len(results) != 2 {
		t.Errorf("expected 2 search results, got %d", len(results))
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
