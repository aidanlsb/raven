package checksvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestDetectNonCanonicalIssues_NoDirectoriesConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.VaultConfig{}
	docs := []*parser.ParsedDocument{
		{
			FilePath: "objects/person/john.md",
			Objects: []*model.Object{
				{ID: "objects/person/john", Type: "person", LineStart: 1},
			},
		},
	}
	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	if len(issues) != 0 {
		t.Fatalf("expected no issues with no directories config; got %#v", issues)
	}
}

func TestDetectNonCanonicalIssues_FlagsTypedObjectOutsideRoot(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	docs := []*parser.ParsedDocument{
		{
			FilePath: "objects/person/john.md",
			Objects: []*model.Object{
				{ID: "objects/person/john", Type: "person", LineStart: 1},
			},
		},
	}

	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	pathIssues := filterByType(issues, check.IssueNonCanonicalPath)
	if len(pathIssues) != 1 {
		t.Fatalf("expected 1 non_canonical_path issue, got %d (%#v)", len(pathIssues), issues)
	}
	got := pathIssues[0]
	if got.Level != check.LevelError {
		t.Fatalf("level = %v, want error", got.Level)
	}
	if !strings.Contains(got.Value, "objects/person/john.md -> type/person/john.md") {
		t.Fatalf("value = %q, expected source -> dest pair", got.Value)
	}
	if got.FilePath != "objects/person/john.md" {
		t.Fatalf("file_path = %q, want objects/person/john.md", got.FilePath)
	}
}

func TestDetectNonCanonicalIssues_AlreadyCanonical(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	docs := []*parser.ParsedDocument{
		{
			FilePath: "type/person/john.md",
			Objects: []*model.Object{
				{ID: "person/john", Type: "person", LineStart: 1},
			},
		},
	}
	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	if filterByType(issues, check.IssueNonCanonicalPath) != nil {
		t.Fatalf("expected no path issues for canonical file; got %#v", issues)
	}
}

func TestDetectNonCanonicalIssues_SkipsDailyAndProtected(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	cfg.Directories.Daily = "daily/"
	cfg.ProtectedPrefixes = []string{"inbox/"}

	docs := []*parser.ParsedDocument{
		{FilePath: "daily/2026-04-19.md", Objects: []*model.Object{{ID: "daily/2026-04-19", Type: "page", LineStart: 1}}},
		{FilePath: "inbox/quick-note.md", Objects: []*model.Object{{ID: "inbox/quick-note", Type: "person", LineStart: 1}}},
		{FilePath: ".trash/old.md", Objects: []*model.Object{{ID: ".trash/old", Type: "person", LineStart: 1}}},
	}
	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	if filterByType(issues, check.IssueNonCanonicalPath) != nil {
		t.Fatalf("expected exempt files to be skipped, got %#v", issues)
	}
}

func TestDetectNonCanonicalIssues_PageOutsidePagesRoot(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	docs := []*parser.ParsedDocument{
		{
			FilePath: "pages/old-note.md",
			Objects: []*model.Object{
				{ID: "pages/old-note", Type: "page", LineStart: 1},
			},
		},
	}
	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	pathIssues := filterByType(issues, check.IssueNonCanonicalPath)
	if len(pathIssues) != 1 {
		t.Fatalf("expected 1 path issue, got %#v", issues)
	}
	if !strings.Contains(pathIssues[0].Value, "page/old-note.md") {
		t.Fatalf("value = %q, expected destination under page/", pathIssues[0].Value)
	}
}

func TestDetectNonCanonicalIssues_RefIncludesRootPrefix(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	docs := []*parser.ParsedDocument{
		{
			FilePath: "type/notes/today.md",
			Refs: []*model.Reference{
				{TargetRaw: "type/person/john", Line: model.IntPtr(5)},
				{TargetRaw: "person/freya", Line: model.IntPtr(6)},
				{TargetRaw: "page/welcome", Line: model.IntPtr(7)},
			},
		},
	}
	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	refIssues := filterByType(issues, check.IssueNonCanonicalRef)
	if len(refIssues) != 2 {
		t.Fatalf("expected 2 ref issues (root-prefixed only), got %d (%#v)", len(refIssues), issues)
	}
	for _, issue := range refIssues {
		if issue.Level != check.LevelWarning {
			t.Fatalf("level = %v, want warning", issue.Level)
		}
		if !strings.HasPrefix(issue.Value, "type/") && !strings.HasPrefix(issue.Value, "page/") {
			t.Fatalf("value = %q does not look root-prefixed", issue.Value)
		}
	}
}

func TestDetectNonCanonicalIssues_DailyDirectoryTypeMismatch(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	cfg.Directories.Daily = "daily/"
	docs := []*parser.ParsedDocument{
		{
			FilePath: "daily/2026-04-19.md",
			Objects: []*model.Object{
				{ID: "daily/2026-04-19", Type: "page", LineStart: 1},
			},
		},
	}

	issues := detectNonCanonicalIssues(docs, schemaWithPerson(), cfg)
	typeIssues := filterByType(issues, check.IssueDirectoryTypeMismatch)
	if len(typeIssues) != 1 {
		t.Fatalf("expected 1 directory_type_mismatch issue, got %#v", issues)
	}
	got := typeIssues[0]
	if got.Level != check.LevelError {
		t.Fatalf("level = %v, want error", got.Level)
	}
	if got.Value != "page -> date" {
		t.Fatalf("value = %q, want page -> date", got.Value)
	}
	if !strings.Contains(got.FixCommand, "rvn reclassify daily/2026-04-19 date --confirm") {
		t.Fatalf("fix command = %q", got.FixCommand)
	}
}

func TestDetectNonCanonicalIssues_DefaultPathTypeMismatch(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	docs := []*parser.ParsedDocument{
		{
			FilePath: "type/person/freya.md",
			Objects: []*model.Object{
				{ID: "person/freya", Type: "project", LineStart: 1},
			},
		},
	}

	issues := detectNonCanonicalIssues(docs, schemaWithPersonProject(), cfg)
	typeIssues := filterByType(issues, check.IssueDirectoryTypeMismatch)
	if len(typeIssues) != 1 {
		t.Fatalf("expected 1 directory_type_mismatch issue, got %#v", issues)
	}
	if got := typeIssues[0].Value; got != "project -> person" {
		t.Fatalf("value = %q, want project -> person", got)
	}
}

func TestCanonicalDestinationPath_PageStripsNestedDirs(t *testing.T) {
	t.Parallel()

	dest, ok := canonicalDestinationPath("pages/sub/old-note.md", "page", true, "page/", schemaWithPerson())
	if !ok {
		t.Fatal("expected canonical destination to be computed for page")
	}
	if dest != "page/old-note.md" {
		t.Fatalf("dest = %q, want page/old-note.md", dest)
	}
}

func TestCanonicalDestinationPath_TypedObjectUsesDefaultPath(t *testing.T) {
	t.Parallel()

	dest, ok := canonicalDestinationPath("objects/person/sub/john.md", "person", false, "type/", schemaWithPerson())
	if !ok {
		t.Fatal("expected canonical destination for typed object with default_path")
	}
	if dest != "type/person/sub/john.md" {
		t.Fatalf("dest = %q, want type/person/sub/john.md", dest)
	}
}

func TestCanonicalDestinationPath_TypedObjectMissingDefaultPathInPath(t *testing.T) {
	t.Parallel()

	if _, ok := canonicalDestinationPath("orphans/john.md", "person", false, "type/", schemaWithPerson()); ok {
		t.Fatal("expected no canonical destination when default_path is missing from current path")
	}
}

func filterByType(issues []check.Issue, t check.IssueType) []check.Issue {
	var out []check.Issue
	for _, issue := range issues {
		if issue.Type == t {
			out = append(out, issue)
		}
	}
	return out
}

func schemaWithPerson() *schema.Schema {
	return &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {DefaultPath: "person/"},
		},
	}
}

func schemaWithPersonProject() *schema.Schema {
	return &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person":  {DefaultPath: "person/"},
			"project": {DefaultPath: "project/"},
		},
	}
}

func configWithDirs(object, page string) *config.VaultConfig {
	return &config.VaultConfig{
		Directories: &config.DirectoriesConfig{
			Object: object,
			Page:   page,
		},
	}
}
