package checksvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestApplyFixes_ReportsSkippedFixes(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithFile("projects/roadmap.md", `---
type: project
title: Roadmap
owner: "[[people/freya]]"
---`).
		Build()

	result, err := ApplyFixes(vault.Path, []FixableIssue{
		{
			FilePath:    "projects/roadmap.md",
			Line:        4,
			IssueType:   check.IssueShortRefCouldBeFullPath,
			FixType:     FixTypeWikilink,
			OldValue:    "freya",
			NewValue:    "people/freya",
			Description: "[[freya]] -> [[people/freya]]",
		},
	}, nil, nil)
	if err != nil {
		t.Fatalf("ApplyFixes returned error: %v", err)
	}
	if result.IssueCount != 0 {
		t.Fatalf("issue count = %d, want 0", result.IssueCount)
	}
	if result.FileCount != 0 {
		t.Fatalf("file count = %d, want 0", result.FileCount)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("skipped = %v, want 1 skipped fix", result.Skipped)
	}
	if got := result.Skipped[0].Reason; got != "expected content no longer present in file" {
		t.Fatalf("skip reason = %q, want expected-content message", got)
	}
}

func TestCollectFixableIssues_IgnoresNilTraitDefinition(t *testing.T) {
	t.Parallel()

	issues := []check.Issue{
		{
			Type:     check.IssueInvalidEnumValue,
			FilePath: "notes/test.md",
			Line:     3,
			Value:    `"high"`,
			Message:  "Invalid value '\"high\"' for trait '@priority'",
		},
	}
	sch := schema.New()
	sch.Traits["priority"] = nil

	fixes := CollectFixableIssues(issues, nil, sch, nil)
	if len(fixes) != 0 {
		t.Fatalf("expected no fixes for nil trait definition, got %#v", fixes)
	}
}
