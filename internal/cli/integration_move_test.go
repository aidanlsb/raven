//go:build integration

package cli_test

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

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
	result := v.RunCLI("move", "people/alice", "people/alice-archived", "--confirm")
	result.MustSucceed(t)

	// Verify the move
	v.AssertFileNotExists("people/alice.md")
	v.AssertFileExists("people/alice-archived.md")

	// Verify the reference was updated in the project
	v.AssertFileContains("projects/website.md", "[[people/alice-archived]]")
}

func TestIntegration_MoveDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()

	v.RunCLI("new", "person", "Preview Me").MustSucceed(t)
	v.RunCLI("new", "project", "Preview Ref", "--field", "owner=[[people/preview-me]]").MustSucceed(t)

	result := v.RunCLI("move", "people/preview-me", "people/preview-me-archived", "--dry-run")
	result.MustSucceed(t)
	if got := result.DataString("status"); got != "preview" {
		t.Fatalf("expected preview status, got %q; raw: %s", got, result.RawJSON)
	}
	if preview, ok := result.Data["preview"].(bool); !ok || !preview {
		t.Fatalf("expected preview=true, got %#v; raw: %s", result.Data["preview"], result.RawJSON)
	}

	v.AssertFileExists("people/preview-me.md")
	v.AssertFileNotExists("people/preview-me-archived.md")
	v.AssertFileContains("projects/preview-ref.md", "[[people/preview-me]]")
}

// TestIntegration_MoveRenamesSectionHeading verifies that moving a section ID
// renames the heading and rewrites inbound fragment references.
func TestIntegration_MoveRenamesSectionHeading(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/website.md", `---
type: project
title: Website
status: active
---

## Tasks

- ship the thing

## Notes
`).
		WithFile("notes/planning.md", `Check [[projects/website#tasks]] and [[projects/website#tasks|the list]].
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("move", "projects/website#tasks", "Completed Tasks")
	result.MustSucceed(t)
	if got := result.DataString("destination"); got != "projects/website#completed-tasks" {
		t.Fatalf("destination = %q, want projects/website#completed-tasks; raw: %s", got, result.RawJSON)
	}

	v.AssertFileContains("projects/website.md", "## Completed Tasks")
	v.AssertFileContains("notes/planning.md", "[[projects/website#completed-tasks]]")
	v.AssertFileContains("notes/planning.md", "[[projects/website#completed-tasks|the list]]")

	// The index should be updated: the new section ID resolves and has backlinks.
	backlinks := v.RunCLI("backlinks", "projects/website#completed-tasks")
	backlinks.MustSucceed(t)
	if !strings.Contains(backlinks.RawJSON, "notes/planning") {
		t.Fatalf("expected backlinks from notes/planning, got: %s", backlinks.RawJSON)
	}

	// check should not report stale fragments after the rename.
	check := v.RunCLI("check")
	check.MustSucceed(t)
	if strings.Contains(check.RawJSON, "stale_fragment") {
		t.Fatalf("unexpected stale_fragment issues after rename: %s", check.RawJSON)
	}
}

// TestIntegration_MoveSectionDryRunDoesNotMutate verifies section rename previews.
func TestIntegration_MoveSectionDryRunDoesNotMutate(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/website.md", `---
type: project
title: Website
status: active
---

## Tasks
`).
		WithFile("notes/planning.md", `Check [[projects/website#tasks]].
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("move", "projects/website#tasks", "Completed Tasks", "--dry-run")
	result.MustSucceed(t)
	if got := result.DataString("destination"); got != "projects/website#completed-tasks" {
		t.Fatalf("destination = %q, want projects/website#completed-tasks; raw: %s", got, result.RawJSON)
	}

	v.AssertFileContains("projects/website.md", "## Tasks")
	v.AssertFileContains("notes/planning.md", "[[projects/website#tasks]]")
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
	result := v.RunCLI("move", "people/alice", "people/alice-archived", "--confirm")
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
	result := v.RunCLI("move", "alice", "people/alice-archived", "--confirm")
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

	result := v.RunCLI("move", "spec/raven-move-friction", "note/", "--confirm")
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

	result := v.RunCLI("move", "doc/test-note", "objects/doc/test-note-moved", "--confirm")
	result.MustSucceed(t)

	v.AssertFileNotExists("objects/doc/test-note.md")
	v.AssertFileExists("objects/doc/test-note-moved.md")
	v.AssertFileNotExists("objects/objects/doc/test-note-moved.md")

	if got := result.DataString("destination"); got != "doc/test-note-moved" {
		t.Fatalf("expected destination object ID %q, got %q", "doc/test-note-moved", got)
	}
}
