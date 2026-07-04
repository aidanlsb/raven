package objectsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
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

func TestMoveByReferenceRenamesSectionAndRewritesRefs(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		WithFile("notes/ref.md", "See [[projects/site#tasks]] and [[projects/site#tasks|the tasks]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md", "notes/ref.md")

	result, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#tasks",
		Destination:    "Completed Tasks",
		UpdateRefs:     true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("MoveByReference() error = %v", err)
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

func TestMoveByReferenceSectionPreviewDoesNotWrite(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", renameSectionProjectContent).
		WithFile("notes/ref.md", "See [[projects/site#tasks]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md", "notes/ref.md")

	result, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#tasks",
		Destination:    "Completed Tasks",
		UpdateRefs:     true,
		Preview:        true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("MoveByReference() error = %v", err)
	}
	if result.DestinationID != "projects/site#completed-tasks" {
		t.Errorf("DestinationID = %q, want projects/site#completed-tasks", result.DestinationID)
	}
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

func TestMoveByReferenceSectionDuplicateSlugFails(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n\n## Notes\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md")

	_, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#tasks",
		Destination:    "Notes",
		UpdateRefs:     true,
		FailOnIndexErr: true,
	})
	if err == nil {
		t.Fatal("expected duplicate slug error, got nil")
	}
	if !strings.Contains(err.Error(), "duplicate section slug") {
		t.Fatalf("error = %v, want duplicate section slug", err)
	}
	if got := v.ReadFile("projects/site.md"); !strings.Contains(got, "## Tasks") {
		t.Fatalf("file modified on failed rename:\n%s", got)
	}
}

func TestMoveByReferenceSectionRejectsNonTitleDestinations(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md")

	for _, destination := range []string{"## Completed", "#done", "projects/site#done"} {
		_, err := MoveByReference(MoveByReferenceRequest{
			VaultPath:      v.Path,
			VaultConfig:    config.DefaultVaultConfig(),
			Schema:         sch,
			Reference:      "projects/site#tasks",
			Destination:    destination,
			UpdateRefs:     true,
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

func TestMoveByReferenceSectionSameSlugUpdatesTitleOnly(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", "---\ntype: project\ntitle: Site\nstatus: active\n---\n\n## Tasks\n").
		WithFile("notes/ref.md", "See [[projects/site#tasks]].\n").
		Build()

	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md", "notes/ref.md")

	result, err := MoveByReference(MoveByReferenceRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#tasks",
		Destination:    "TASKS",
		UpdateRefs:     true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("MoveByReference() error = %v", err)
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
