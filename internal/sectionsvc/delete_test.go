package sectionsvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/testutil"
)

const deleteSectionOutline = `---
type: project
title: Site
status: active
---

# Project

Intro

## Alpha

Alpha body

### Alpha Child

Child body with [[projects/site#alpha]] and [[projects/site#alpha-child]].

## Beta

Beta body links to [[projects/site#alpha-child]].
`

func TestDeletePreviewReportsExactSubtreeAndInboundReferences(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", deleteSectionOutline).
		WithFile("notes/ref.md", "See [[projects/site#alpha]] and [[projects/site#alpha-child|child]].\n").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md", "notes/ref.md")

	result, err := Delete(DeleteRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#alpha",
		Preview:        true,
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if result.SectionID != "projects/site#alpha" {
		t.Fatalf("SectionID = %q, want projects/site#alpha", result.SectionID)
	}
	if result.LineStart != 11 || result.LineEnd != 18 {
		t.Fatalf("line range = %d-%d, want 11-18", result.LineStart, result.LineEnd)
	}
	wantRemoved := "## Alpha\n\nAlpha body\n\n### Alpha Child\n\nChild body with [[projects/site#alpha]] and [[projects/site#alpha-child]].\n"
	if result.RemovedContent != wantRemoved {
		t.Fatalf("RemovedContent = %q, want %q", result.RemovedContent, wantRemoved)
	}
	if got := strings.Join(result.DeletedSections, ","); got != "projects/site#alpha,projects/site#alpha-child" {
		t.Fatalf("DeletedSections = %q", got)
	}

	if len(result.Backlinks) != 3 {
		t.Fatalf("Backlinks = %#v, want three references outside the deleted subtree", result.Backlinks)
	}
	for _, backlink := range result.Backlinks {
		if backlink.Line != nil && backlink.FilePath == "projects/site.md" && *backlink.Line >= result.LineStart && *backlink.Line <= result.LineEnd {
			t.Fatalf("Backlinks includes reference removed with subtree: %#v", backlink)
		}
	}
	if got := v.ReadFile("projects/site.md"); got != deleteSectionOutline {
		t.Fatalf("preview changed source file:\n%s", got)
	}
}

func TestDeleteApplyRemovesOnlySubtreeAndLeavesReportedReferences(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", deleteSectionOutline).
		WithFile("notes/ref.md", "See [[projects/site#alpha]] and [[projects/site#alpha-child|child]].\n").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md", "notes/ref.md")

	result, err := Delete(DeleteRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#alpha",
		FailOnIndexErr: true,
	})
	if err != nil {
		t.Fatalf("Delete() error = %v", err)
	}
	if len(result.Backlinks) != 3 {
		t.Fatalf("Backlinks = %#v, want three reported stale references", result.Backlinks)
	}

	content := v.ReadFile("projects/site.md")
	for _, removed := range []string{"## Alpha", "### Alpha Child", "Alpha body", "Child body"} {
		if strings.Contains(content, removed) {
			t.Fatalf("deleted subtree content %q remains:\n%s", removed, content)
		}
	}
	for _, preserved := range []string{"# Project", "Intro", "## Beta", "Beta body links to [[projects/site#alpha-child]]."} {
		if !strings.Contains(content, preserved) {
			t.Fatalf("sibling/parent content %q was not preserved:\n%s", preserved, content)
		}
	}
	if got := v.ReadFile("notes/ref.md"); got != "See [[projects/site#alpha]] and [[projects/site#alpha-child|child]].\n" {
		t.Fatalf("inbound references were changed without a safe replacement:\n%s", got)
	}
}

func TestDeleteRejectsNonSectionReferences(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", deleteSectionOutline).
		WithFile("assets/paper.pdf", "pdf").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md")

	for _, reference := range []string{"projects/site", "projects/site.md", "assets/paper.pdf"} {
		t.Run(reference, func(t *testing.T) {
			_, err := Delete(DeleteRequest{
				VaultPath:      v.Path,
				VaultConfig:    config.DefaultVaultConfig(),
				Schema:         sch,
				Reference:      reference,
				Preview:        true,
				FailOnIndexErr: true,
			})
			if err == nil || !strings.Contains(err.Error(), "section reference required") {
				t.Fatalf("Delete(%q) error = %v, want section-only rejection", reference, err)
			}
		})
	}
	if got := v.ReadFile("projects/site.md"); got != deleteSectionOutline {
		t.Fatalf("rejected delete changed file:\n%s", got)
	}
}

func TestDeleteRejectsSurvivingSlugShift(t *testing.T) {
	t.Parallel()

	content := `---
type: project
title: Site
status: active
---

## Repeat

First

## Repeat

Second
`
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/site.md", content).
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "projects/site.md")

	_, err := Delete(DeleteRequest{
		VaultPath:      v.Path,
		VaultConfig:    config.DefaultVaultConfig(),
		Schema:         sch,
		Reference:      "projects/site#repeat",
		Preview:        true,
		FailOnIndexErr: true,
	})
	if err == nil || !strings.Contains(err.Error(), "shift slug") {
		t.Fatalf("Delete() error = %v, want slug-shift rejection", err)
	}
	if got := v.ReadFile("projects/site.md"); got != content {
		t.Fatalf("failed delete changed file:\n%s", got)
	}
}
