//go:build integration

package cli_test

import (
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

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

func TestIntegration_TraitUpdateExplicitTraitIDsPreviewAndApply(t *testing.T) {
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

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("update",
		"--trait-id", "tasks/task1.md:trait:0",
		"--trait-id", "tasks/task1.md:trait:1",
		"high",
	)
	result.MustSucceed(t)
	if preview, ok := result.Data["preview"].(bool); !ok || !preview {
		t.Fatalf("expected preview=true, got %#v; raw: %s", result.Data["preview"], result.RawJSON)
	}
	if total, ok := result.Data["total"].(float64); !ok || total != 2 {
		t.Fatalf("expected total=2, got %#v; raw: %s", result.Data["total"], result.RawJSON)
	}
	v.AssertFileContains("tasks/task1.md", "@priority(low) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(low) Second task")

	result = v.RunCLI("update",
		"--trait-id", "tasks/task1.md:trait:0",
		"--trait-id", "tasks/task1.md:trait:1",
		"--confirm",
		"high",
	)
	result.MustSucceed(t)
	v.AssertFileContains("tasks/task1.md", "@priority(high) First task")
	v.AssertFileContains("tasks/task1.md", "@priority(high) Second task")
}

func TestIntegration_TraitUpdateRejectsStdinAndExplicitTraitIDs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	result := v.RunCLI("update", "--stdin", "--trait-id", "tasks/task1.md:trait:0", "high")
	result.MustFail(t, "INVALID_INPUT")
	result.MustFailWithMessage(t, "mutually exclusive")
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

func TestIntegration_TraitBulkUpdateValidationFailureIncludesAttemptedIDs(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types: {}
traits:
  todo:
    type: bool
`).
		WithFile("tasks/first.md", "- First @todo(false)\n- Second @todo(false)\n").
		WithFile("tasks/second.md", "- Third @todo(false)\n").
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	attempted := []string{
		"tasks/second.md:trait:0",
		"tasks/first.md:trait:1",
		"tasks/first.md:trait:0",
	}
	stdin := attempted[0] + "\n" + attempted[1] + "\n" + attempted[2] + "\n"
	tests := []struct {
		name string
		args []string
	}{
		{name: "preview", args: []string{"update", "--stdin", "done"}},
		{name: "apply", args: []string{"update", "--stdin", "done", "--confirm"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := v.RunCLIWithStdin(stdin, tt.args...)
			result.MustFail(t, "VALIDATION_FAILED")
			if result.Error == nil {
				t.Fatalf("missing error envelope: %s", result.RawJSON)
			}

			gotIDs, ok := result.Error.Details["trait_ids"].([]interface{})
			if !ok {
				t.Fatalf("trait_ids = %#v, want ordered array; raw: %s", result.Error.Details["trait_ids"], result.RawJSON)
			}
			if len(gotIDs) != len(attempted) {
				t.Fatalf("trait_ids count = %d, want %d; raw: %s", len(gotIDs), len(attempted), result.RawJSON)
			}
			for i, want := range attempted {
				if gotIDs[i] != want {
					t.Fatalf("trait_ids[%d] = %#v, want %q; raw: %s", i, gotIDs[i], want, result.RawJSON)
				}
			}
			if total, ok := result.Error.Details["total"].(float64); !ok || int(total) != len(attempted) {
				t.Fatalf("total = %#v, want %d; raw: %s", result.Error.Details["total"], len(attempted), result.RawJSON)
			}
		})
	}

	v.AssertFileContains("tasks/first.md", "@todo(false)")
	v.AssertFileContains("tasks/second.md", "@todo(false)")
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
