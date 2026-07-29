package checkfixsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
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

func TestApplyFixes_TextFixesAreLineScoped(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		content string
		fixes   []FixableIssue
		want    string
	}{
		{
			name:    "identical replacements on different lines",
			content: "First: [[freya]]\nSecond: [[freya]]\n",
			fixes: []FixableIssue{
				{
					FilePath:  "notes/refs.md",
					Line:      1,
					IssueType: check.IssueShortRefCouldBeFullPath,
					FixType:   FixTypeWikilink,
					OldValue:  "freya",
					NewValue:  "people/freya",
				},
				{
					FilePath:  "notes/refs.md",
					Line:      2,
					IssueType: check.IssueShortRefCouldBeFullPath,
					FixType:   FixTypeWikilink,
					OldValue:  "freya",
					NewValue:  "people/freya",
				},
			},
			want: "First: [[people/freya]]\nSecond: [[people/freya]]\n",
		},
		{
			name:    "conflicting replacements on different lines",
			content: "First: [[shared]]\nSecond: [[shared]]\n",
			fixes: []FixableIssue{
				{
					FilePath:  "notes/refs.md",
					Line:      1,
					IssueType: check.IssueShortRefCouldBeFullPath,
					FixType:   FixTypeWikilink,
					OldValue:  "shared",
					NewValue:  "people/first",
				},
				{
					FilePath:  "notes/refs.md",
					Line:      2,
					IssueType: check.IssueShortRefCouldBeFullPath,
					FixType:   FixTypeWikilink,
					OldValue:  "shared",
					NewValue:  "people/second",
				},
			},
			want: "First: [[people/first]]\nSecond: [[people/second]]\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vault := testutil.NewTestVault(t).
				WithFile("notes/refs.md", tt.content).
				Build()

			result, err := ApplyFixes(vault.Path, tt.fixes, nil, nil)
			if err != nil {
				t.Fatalf("ApplyFixes returned error: %v", err)
			}
			if result.IssueCount != 2 {
				t.Errorf("issue count = %d, want 2", result.IssueCount)
			}
			if result.FileCount != 1 {
				t.Errorf("file count = %d, want 1", result.FileCount)
			}
			if len(result.Skipped) != 0 {
				t.Errorf("skipped = %v, want none", result.Skipped)
			}
			if got := vault.ReadFile("notes/refs.md"); got != tt.want {
				t.Errorf("content = %q, want %q", got, tt.want)
			}
		})
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

func TestCollectFixableIssues_NonCanonicalPath_BuildsMoveFix(t *testing.T) {
	t.Parallel()

	cfg := fixTestConfigWithDirs("type/", "page/")
	issues := []check.Issue{
		{
			Type:     check.IssueNonCanonicalPath,
			FilePath: "objects/person/john.md",
			Value:    "objects/person/john.md -> type/person/john.md",
		},
	}
	fixes := CollectFixableIssues(issues, nil, fixTestSchemaWithPerson(), cfg)
	if len(fixes) != 1 {
		t.Fatalf("expected 1 move fix, got %#v", fixes)
	}
	fix := fixes[0]
	if fix.FixType != FixTypeMoveFile {
		t.Fatalf("fix_type = %v, want move_file", fix.FixType)
	}
	if fix.NewFilePath != "type/person/john.md" {
		t.Fatalf("new_file_path = %q, want type/person/john.md", fix.NewFilePath)
	}
	if fix.SourceObjectID == "" || fix.DestObjectID == "" {
		t.Fatalf("expected resolved source/dest object IDs, got src=%q dest=%q", fix.SourceObjectID, fix.DestObjectID)
	}
}

func TestCollectFixableIssues_NonCanonicalRef_BuildsWikilinkFix(t *testing.T) {
	t.Parallel()

	cfg := fixTestConfigWithDirs("type/", "page/")
	issues := []check.Issue{
		{
			Type:     check.IssueNonCanonicalRef,
			FilePath: "type/notes/today.md",
			Line:     5,
			Value:    "type/person/john",
		},
	}
	fixes := CollectFixableIssues(issues, nil, fixTestSchemaWithPerson(), cfg)
	if len(fixes) != 1 {
		t.Fatalf("expected 1 ref fix, got %#v", fixes)
	}
	fix := fixes[0]
	if fix.FixType != FixTypeWikilink {
		t.Fatalf("fix_type = %v, want wikilink", fix.FixType)
	}
	if fix.OldValue != "type/person/john" || fix.NewValue != "person/john" {
		t.Fatalf("old/new = %q/%q, want type/person/john -> person/john", fix.OldValue, fix.NewValue)
	}
}

func fixTestSchemaWithPerson() *schema.Schema {
	return &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {DefaultPath: "person/"},
		},
	}
}

func fixTestConfigWithDirs(object, page string) *config.VaultConfig {
	return &config.VaultConfig{
		Directories: &config.DirectoriesConfig{
			Object: object,
			Page:   page,
		},
	}
}
