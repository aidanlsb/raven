package sectionsvc

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
)

const renameSectionProjectContent = `---
type: project
title: Site
status: active
---

## Tasks

- do a thing

## Notes

Cross ref [[projects/site#tasks]] here.
`

func TestRenameRewritesInboundReferences(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		WithFile("notes/ref.md", "See [[projects/site#tasks]] and [[projects/site#tasks|the tasks]].\n").
		Build()

	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md")

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "Completed Tasks",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}
	if result.SourceID != "projects/site#tasks" {
		t.Errorf("SourceID = %q, want projects/site#tasks", result.SourceID)
	}
	if result.DestinationID != "projects/site#completed-tasks" {
		t.Errorf("DestinationID = %q, want projects/site#completed-tasks", result.DestinationID)
	}

	site := v.ReadFile("projects/site.md")
	if !strings.Contains(site, "## Completed Tasks") {
		t.Fatalf("heading not renamed:\n%s", site)
	}
	if !strings.Contains(site, "[[projects/site#completed-tasks]]") {
		t.Fatalf("same-file ref not rewritten:\n%s", site)
	}
	ref := v.ReadFile("notes/ref.md")
	if !strings.Contains(ref, "[[projects/site#completed-tasks]]") || !strings.Contains(ref, "[[projects/site#completed-tasks|the tasks]]") {
		t.Fatalf("inbound refs not rewritten:\n%s", ref)
	}
	if strings.Contains(site+ref, "#tasks]]") {
		t.Fatalf("stale fragment refs remain:\nsite:\n%s\nref:\n%s", site, ref)
	}
	assertChangeSetChanged(t, result.ChangeSet, "projects/site.md", "notes/ref.md")

	wantUpdated := map[string]bool{"projects/site": false, "notes/ref": false}
	for _, id := range result.UpdatedRefs {
		if _, ok := wantUpdated[id]; ok {
			wantUpdated[id] = true
		}
	}
	for id, seen := range wantUpdated {
		if !seen {
			t.Errorf("UpdatedRefs missing %q: %#v", id, result.UpdatedRefs)
		}
	}
}

func TestRenameRewritesReferencesThroughSharedRewriter(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
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
        values: [active, paused, done]
      related:
        type: ref
        target: project
`).
		WithFile("projects/site.md", `---
type: project
title: Site
status: active
---

## Tasks

- do a thing
`).
		WithFile("notes/ref.md", "Body [[projects/site#tasks]], alias [[projects/site#tasks|the tasks]], and Markdown [tasks](projects/site#tasks) / [angle](<projects/site#tasks>).\n\n```markdown\n[[projects/site#tasks]] and [tasks](projects/site#tasks)\n```\n").
		WithFile("projects/consumer.md", `---
type: project
title: Consumer
status: active
related: projects/site#tasks
---
`).
		Build()

	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md", "projects/consumer.md")

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "Completed Tasks",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if len(result.WarningMessages) != 0 {
		t.Fatalf("unexpected warnings: %#v", result.WarningMessages)
	}

	ref := v.ReadFile("notes/ref.md")
	if !strings.Contains(ref, "[[projects/site#completed-tasks]]") {
		t.Fatalf("markdown body ref not rewritten:\n%s", ref)
	}
	if !strings.Contains(ref, "[[projects/site#completed-tasks|the tasks]]") {
		t.Fatalf("alias ref not rewritten:\n%s", ref)
	}
	if !strings.Contains(ref, "[tasks](projects/site#completed-tasks)") ||
		!strings.Contains(ref, "[angle](<projects/site#completed-tasks>)") {
		t.Fatalf("Markdown refs not rewritten:\n%s", ref)
	}
	if !strings.Contains(ref, "```markdown\n[[projects/site#tasks]] and [tasks](projects/site#tasks)\n```") {
		t.Fatalf("fenced-code ref was rewritten:\n%s", ref)
	}

	consumer := v.ReadFile("projects/consumer.md")
	if !strings.Contains(consumer, "related: projects/site#completed-tasks") {
		t.Fatalf("frontmatter ref not rewritten:\n%s", consumer)
	}
	assertChangeSetChanged(t, result.ChangeSet, "projects/site.md", "notes/ref.md", "projects/consumer.md")
}

func TestRenamePreservesPostCommitResolutionErrors(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		WithFile("notes/ref.md", "See [[projects/site#tasks]].\n").
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md")

	db, err := index.Open(v.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if _, err := db.DB().Exec(`
		CREATE TRIGGER fail_reference_resolution
		BEFORE UPDATE OF target_id ON refs
		BEGIN
			SELECT RAISE(ABORT, 'reference resolution failed');
		END;
	`); err != nil {
		_ = db.Close()
		t.Fatalf("create resolution trigger: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "Completed Tasks",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	assertChangeSetChanged(t, result.ChangeSet, "projects/site.md", "notes/ref.md")

	projected := reindexsvc.ProjectChanges(rt, result.ChangeSet, "")
	if len(projected.Warnings) == 0 {
		t.Fatalf("projection warnings = %#v, want post-commit resolution failure", projected.Warnings)
	}
	var resolutionErr *index.PostCommitReferenceResolutionError
	foundVaultWide := false
	for _, warning := range projected.Warnings {
		if errors.As(warning.Err, &resolutionErr) && resolutionErr.VaultWide {
			foundVaultWide = true
			break
		}
	}
	if !foundVaultWide {
		t.Fatalf("projection warnings = %#v, want vault-wide post-commit error", projected.Warnings)
	}
	checkDB, err := index.Open(v.Path)
	if err != nil {
		t.Fatalf("reopen index: %v", err)
	}
	defer checkDB.Close()
	if object, err := checkDB.GetObject("projects/site"); err != nil {
		t.Fatalf("get committed project: %v", err)
	} else if object == nil {
		t.Fatal("renamed project is absent from committed index")
	}
}

func TestRenamePreviewDoesNotWrite(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		WithFile("notes/ref.md", "See [[projects/site#tasks]].\n").
		Build()

	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md")

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "Completed Tasks",
		Preview:        true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if result.DestinationID != "projects/site#completed-tasks" {
		t.Errorf("DestinationID = %q, want projects/site#completed-tasks", result.DestinationID)
	}
	assertChangeSetEmpty(t, result.ChangeSet)
	if len(result.UpdatedRefs) == 0 {
		t.Errorf("preview UpdatedRefs = %#v, want planned updates", result.UpdatedRefs)
	}
	if got := v.ReadFile("projects/site.md"); got != renameSectionProjectContent {
		t.Fatalf("preview modified source file:\n%s", got)
	}
	if got := v.ReadFile("notes/ref.md"); !strings.Contains(got, "[[projects/site#tasks]]") {
		t.Fatalf("preview modified ref file:\n%s", got)
	}
}

func TestRenameRejectsSectionSlugCollisions(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		content     string
		reference   string
		wantHeading string
	}{
		{
			name:        "renamed section would receive suffix",
			content:     "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Notes\n\n## Tasks\n",
			reference:   "projects/site#tasks",
			wantHeading: "## Tasks",
		},
		{
			name:        "rename would shift existing section",
			content:     "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n\n## Notes\n",
			reference:   "projects/site#tasks",
			wantHeading: "## Tasks",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("projects/site.md", tt.content).
				Build()
			rt := testRuntime(t, v.Path)
			indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

			_, err := Rename(rt, RenameRequest{
				Reference:      tt.reference,
				NewHeadingText: "Notes",
				FailOnIndexErr: true,
			})
			if err == nil {
				t.Fatal("expected duplicate slug error, got nil")
			}
			if !strings.Contains(err.Error(), "duplicate section slug") {
				t.Fatalf("error = %v, want duplicate section slug", err)
			}
			if got := v.ReadFile("projects/site.md"); !strings.Contains(got, tt.wantHeading) {
				t.Fatalf("file modified on failed rename:\n%s", got)
			}
		})
	}
}

func TestRenameRejectsNonTitleDestinations(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n").
		Build()

	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	for _, destination := range []string{"## Completed", "#done", "projects/site#done", "projects/other#done"} {
		_, err := Rename(rt, RenameRequest{
			Reference:      "projects/site#tasks",
			NewHeadingText: destination,
			FailOnIndexErr: true,
		})
		if err == nil {
			t.Fatalf("destination %q: expected error, got nil", destination)
		}
		if !strings.Contains(err.Error(), "new heading text") {
			t.Fatalf("destination %q: error = %v, want heading text guidance", destination, err)
		}
	}
}

func TestRenameSameSlugUpdatesTitleOnly(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n").
		WithFile("notes/ref.md", "See [[projects/site#tasks]].\n").
		Build()

	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md")

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "TASKS",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if result.DestinationID != "projects/site#tasks" {
		t.Errorf("DestinationID = %q, want projects/site#tasks", result.DestinationID)
	}
	if len(result.UpdatedRefs) != 0 {
		t.Errorf("UpdatedRefs = %#v, want none for same-slug rename", result.UpdatedRefs)
	}
	if got := v.ReadFile("projects/site.md"); !strings.Contains(got, "## TASKS") {
		t.Fatalf("heading text not updated:\n%s", got)
	}
	if got := v.ReadFile("notes/ref.md"); !strings.Contains(got, "[[projects/site#tasks]]") {
		t.Fatalf("refs should be unchanged:\n%s", got)
	}
}

func TestRenameUsesSharedDocumentLoad(t *testing.T) {
	t.Parallel()

	content := strings.TrimSuffix(renameSectionProjectContent, "\n")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", content).
		WithFile("notes/ref.md", "See [[projects/site#tasks]].").
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md", "notes/ref.md")

	result, err := Rename(rt, RenameRequest{
		Reference:      "projects/site#tasks",
		NewHeadingText: "Completed Tasks",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Rename() error = %v", err)
	}
	if result.DestinationID != "projects/site#completed-tasks" {
		t.Fatalf("DestinationID = %q", result.DestinationID)
	}

	site := v.ReadFile("projects/site.md")
	if strings.HasSuffix(site, "\n") {
		t.Fatalf("loadDocument rewrite added a trailing newline:\n%q", site)
	}
	if !strings.Contains(site, "## Completed Tasks") {
		t.Fatalf("heading not renamed:\n%s", site)
	}
	if !strings.Contains(site, "[[projects/site#completed-tasks]]") {
		t.Fatalf("same-file ref not rewritten:\n%s", site)
	}
	if got := v.ReadFile("notes/ref.md"); got != "See [[projects/site#completed-tasks]]." {
		t.Fatalf("inbound ref file = %q", got)
	}
}

func TestRenameRejectsFileReferences(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	_, err := Rename(rt, RenameRequest{
		Reference:      "projects/site",
		NewHeadingText: "Done",
		FailOnIndexErr: true,
	})
	if err == nil || !strings.Contains(err.Error(), "invalid section ID") {
		t.Fatalf("Rename() error = %v, want invalid section ID", err)
	}
	assertServiceCode(t, err, "INVALID_INPUT")
}

func TestRewriteHeadingLeavesOtherLinesUnchanged(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		Build()
	rt := testRuntime(t, v.Path)
	indexVaultFiles(t, v.Path, rt.Schema, "projects/site.md")

	state, target, err := loadResolvedSection(rt, "projects/site#tasks")
	if err != nil {
		t.Fatalf("loadResolvedSection() error = %v", err)
	}
	updated, renamed, err := rewriteHeading(rt, state, target, "Completed Tasks")
	if err != nil {
		t.Fatalf("rewriteHeading() error = %v", err)
	}
	if renamed == nil || renamed.ID != "projects/site#completed-tasks" {
		t.Fatalf("renamed = %#v", renamed)
	}
	if strings.Contains(updated, "## Tasks\n") {
		t.Fatalf("old heading remains:\n%s", updated)
	}
	if !strings.Contains(updated, "## Notes") || !strings.Contains(updated, "Cross ref [[projects/site#tasks]]") {
		t.Fatalf("rewriteHeading mutated non-heading lines:\n%s", updated)
	}
	if got := v.ReadFile("projects/site.md"); got != renameSectionProjectContent {
		t.Fatal("rewriteHeading wrote the vault file")
	}
}
