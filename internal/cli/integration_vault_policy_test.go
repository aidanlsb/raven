//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

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

	newResult := v.RunCLI("new", "project", "Blocked Project", "--object-path", "private/blocked-project")
	newResult.MustFail(t, "VALIDATION_FAILED")
	newResult.MustFailWithMessage(t, "protected")

	upsertResult := v.RunCLI("upsert", "project", "Blocked Project", "--object-path", "private/blocked-project", "--content", "# blocked")
	upsertResult.MustFail(t, "VALIDATION_FAILED")
	upsertResult.MustFailWithMessage(t, "protected")

	addResult := v.RunCLI("add", "Protected note", "--to", "private/notes.md")
	addResult.MustFail(t, "VALIDATION_FAILED")
	addResult.MustFailWithMessage(t, "protected")
	v.AssertFileNotContains("private/notes.md", "Protected note")

	setResult := v.RunCLI("set", "private/task.md", "--field", "status=done")
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

func TestIntegration_ExcludeRejectsMutationCommands(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("exclude:\n  - private/\n").
		WithFile("private/notes.md", "# Notes\nold task\n").
		Build()

	editResult := v.RunCLI("edit", "private/notes.md", "old task", "done task")
	editResult.MustFail(t, "VALIDATION_FAILED")
	editResult.MustFailWithMessage(t, "excluded")
	v.AssertFileContains("private/notes.md", "old task")

	addResult := v.RunCLI("add", "new note", "--to", "private/notes.md")
	addResult.MustFail(t, "VALIDATION_FAILED")
	addResult.MustFailWithMessage(t, "excluded")
	v.AssertFileNotContains("private/notes.md", "new note")
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
