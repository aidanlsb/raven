package checksvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
)

func TestIssueCollector_FiltersScopeAndTypeOnce(t *testing.T) {
	t.Parallel()

	personDoc := &parser.ParsedDocument{
		FilePath: "people/freya.md",
		Objects:  []*model.Object{{ID: "people/freya", Type: "person"}},
		Traits:   []*model.Trait{{TraitType: "due"}},
	}
	projectDoc := &parser.ParsedDocument{
		FilePath: "projects/raven.md",
		Objects:  []*model.Object{{ID: "projects/raven", Type: "project"}},
	}

	personIssue := check.Issue{
		Type:     check.IssueUnknownFrontmatter,
		Level:    check.LevelError,
		FilePath: personDoc.FilePath,
		Message:  "unknown key",
	}
	projectIssue := check.Issue{
		Type:     check.IssueMissingReference,
		Level:    check.LevelError,
		FilePath: projectDoc.FilePath,
		Message:  "missing",
		Value:    "people/ghost",
	}
	dueTraitIssue := check.Issue{
		Type:     check.IssueUndefinedTrait,
		Level:    check.LevelWarning,
		FilePath: personDoc.FilePath,
		Message:  "undefined",
		Value:    "due",
	}
	parseIssue := check.Issue{
		Type:     check.IssueParseError,
		Level:    check.LevelError,
		FilePath: "broken.md",
		Message:  "bad yaml",
	}

	tests := []struct {
		name     string
		scope    *Scope
		include  map[check.IssueType]bool
		exclude  map[check.IssueType]bool
		errors   bool
		issues   []check.Issue
		want     []check.IssueType
		wantErr  int
		wantWarn int
	}{
		{
			name:  "type filter keeps matching documents",
			scope: &Scope{Type: "type_filter", Value: "person"},
			issues: []check.Issue{
				personIssue,
				projectIssue,
				dueTraitIssue,
			},
			want:     []check.IssueType{check.IssueUnknownFrontmatter, check.IssueUndefinedTrait},
			wantErr:  1,
			wantWarn: 1,
		},
		{
			name:  "trait filter keeps matching trait issues",
			scope: &Scope{Type: "trait_filter", Value: "due"},
			issues: []check.Issue{
				personIssue,
				dueTraitIssue,
				projectIssue,
			},
			want:     []check.IssueType{check.IssueUnknownFrontmatter, check.IssueUndefinedTrait},
			wantErr:  1,
			wantWarn: 1,
		},
		{
			name:    "include filter drops other types",
			scope:   &Scope{Type: "full"},
			include: map[check.IssueType]bool{check.IssueParseError: true},
			issues:  []check.Issue{personIssue, parseIssue, projectIssue},
			want:    []check.IssueType{check.IssueParseError},
			wantErr: 1,
		},
		{
			name:    "exclude filter drops parse errors",
			scope:   &Scope{Type: "full"},
			exclude: map[check.IssueType]bool{check.IssueParseError: true},
			issues:  []check.Issue{parseIssue, personIssue},
			want:    []check.IssueType{check.IssueUnknownFrontmatter},
			wantErr: 1,
		},
		{
			name:    "errors-only drops warnings",
			scope:   &Scope{Type: "full"},
			errors:  true,
			issues:  []check.Issue{personIssue, dueTraitIssue},
			want:    []check.IssueType{check.IssueUnknownFrontmatter},
			wantErr: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := &RunResult{}
			collector := newIssueCollector(result, tt.include, tt.exclude, tt.errors, tt.scope)
			collector.setDocs([]*parser.ParsedDocument{personDoc, projectDoc})
			collector.addAll(tt.issues)

			if len(collector.issues) != len(tt.want) {
				t.Fatalf("issues = %#v, want types %v", collector.issues, tt.want)
			}
			for i, issue := range collector.issues {
				if issue.Type != tt.want[i] {
					t.Errorf("issue[%d] type = %s, want %s", i, issue.Type, tt.want[i])
				}
			}
			if result.ErrorCount != tt.wantErr {
				t.Errorf("error count = %d, want %d", result.ErrorCount, tt.wantErr)
			}
			if result.WarningCount != tt.wantWarn {
				t.Errorf("warning count = %d, want %d", result.WarningCount, tt.wantWarn)
			}
		})
	}
}

func TestIssueCollector_AddSchemaAllContinuesAfterExcluded(t *testing.T) {
	t.Parallel()

	result := &RunResult{}
	collector := newIssueCollector(result, nil, map[check.IssueType]bool{check.IssueUnusedType: true}, false, &Scope{Type: "full"})
	collector.addSchemaAll([]check.SchemaIssue{
		{Type: check.IssueUnusedType, Level: check.LevelWarning, Value: "person"},
		{Type: check.IssueUnusedTrait, Level: check.LevelWarning, Value: "due"},
		{Type: check.IssueMissingTargetType, Level: check.LevelError, Value: "ghost"},
	})

	if len(collector.schemaIssues) != 2 {
		t.Fatalf("schema issues = %#v, want unused_trait and missing_target_type", collector.schemaIssues)
	}
	if collector.schemaIssues[0].Type != check.IssueUnusedTrait {
		t.Errorf("first kept type = %s, want unused_trait", collector.schemaIssues[0].Type)
	}
	if collector.schemaIssues[1].Type != check.IssueMissingTargetType {
		t.Errorf("second kept type = %s, want missing_target_type", collector.schemaIssues[1].Type)
	}
	if result.WarningCount != 1 || result.ErrorCount != 1 {
		t.Fatalf("counts warn=%d err=%d, want 1 and 1", result.WarningCount, result.ErrorCount)
	}
}

func TestIsSchemaIssueInScope(t *testing.T) {
	t.Parallel()

	issue := check.SchemaIssue{Value: "person.owner"}
	tests := []struct {
		scope *Scope
		want  bool
	}{
		{scope: &Scope{Type: "full"}, want: true},
		{scope: &Scope{Type: "type_filter", Value: "person"}, want: true},
		{scope: &Scope{Type: "type_filter", Value: "project"}, want: false},
		{scope: &Scope{Type: "trait_filter", Value: "person.owner"}, want: true},
		{scope: &Scope{Type: "trait_filter", Value: "due"}, want: false},
	}
	for _, tt := range tests {
		if got := isSchemaIssueInScope(issue, tt.scope); got != tt.want {
			t.Errorf("scope %#v: got %v, want %v", tt.scope, got, tt.want)
		}
	}
}
