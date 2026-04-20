package checksvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestDetectNonCanonicalIssues_NoDirectoriesConfig(t *testing.T) {
	t.Parallel()

	cfg := &config.VaultConfig{}
	docs := []*parser.ParsedDocument{
		{
			FilePath: "objects/person/john.md",
			Objects: []*parser.ParsedObject{
				{ID: "objects/person/john", ObjectType: "person", LineStart: 1},
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
			Objects: []*parser.ParsedObject{
				{ID: "objects/person/john", ObjectType: "person", LineStart: 1},
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
			Objects: []*parser.ParsedObject{
				{ID: "person/john", ObjectType: "person", LineStart: 1},
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
		{FilePath: "daily/2026-04-19.md", Objects: []*parser.ParsedObject{{ID: "daily/2026-04-19", ObjectType: "page", LineStart: 1}}},
		{FilePath: "inbox/quick-note.md", Objects: []*parser.ParsedObject{{ID: "inbox/quick-note", ObjectType: "person", LineStart: 1}}},
		{FilePath: ".trash/old.md", Objects: []*parser.ParsedObject{{ID: ".trash/old", ObjectType: "person", LineStart: 1}}},
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
			Objects: []*parser.ParsedObject{
				{ID: "pages/old-note", ObjectType: "page", LineStart: 1},
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
			Refs: []*parser.ParsedRef{
				{TargetRaw: "type/person/john", Line: 5},
				{TargetRaw: "person/freya", Line: 6},
				{TargetRaw: "page/welcome", Line: 7},
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

func TestCollectFixableIssues_NonCanonicalPath_BuildsMoveFix(t *testing.T) {
	t.Parallel()

	cfg := configWithDirs("type/", "page/")
	issues := []check.Issue{
		{
			Type:     check.IssueNonCanonicalPath,
			FilePath: "objects/person/john.md",
			Value:    "objects/person/john.md -> type/person/john.md",
		},
	}
	fixes := CollectFixableIssues(issues, nil, schemaWithPerson(), cfg)
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

	cfg := configWithDirs("type/", "page/")
	issues := []check.Issue{
		{
			Type:     check.IssueNonCanonicalRef,
			FilePath: "type/notes/today.md",
			Line:     5,
			Value:    "type/person/john",
		},
	}
	fixes := CollectFixableIssues(issues, nil, schemaWithPerson(), cfg)
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

func configWithDirs(object, page string) *config.VaultConfig {
	return &config.VaultConfig{
		Directories: &config.DirectoriesConfig{
			Object: object,
			Page:   page,
		},
	}
}
