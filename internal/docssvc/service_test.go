package docssvc

import (
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestListSectionsFromRootLoadsRepositoryDocs(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(repoRoot(t), "docs")
	sections, err := ListSectionsFromRoot(docsRoot)
	if err != nil {
		t.Fatalf("ListSectionsFromRoot() error = %v", err)
	}
	if len(sections) == 0 {
		t.Fatalf("expected docs sections, got none")
	}

	var ids []string
	for _, section := range sections {
		ids = append(ids, section.ID)
	}
	for _, expected := range []string{"agents", "getting-started", "querying", "types-and-traits", "vault-management"} {
		if !slices.Contains(ids, expected) {
			t.Fatalf("expected section %q in %v", expected, ids)
		}
	}
}

func TestNormalizePathSlug(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "simple", in: "query-language", want: "query-language"},
		{name: "underscore", in: "query_language", want: "query-language"},
		{name: "spaces", in: "Query Language", want: "query-language"},
		{name: "nested", in: "api/Query Language", want: "api/query-language"},
		{name: "windows separators", in: `api\Query Language`, want: "api/query-language"},
		{name: "extra separators", in: "  api//query_language  ", want: "api/query-language"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := NormalizePathSlug(tc.in)
			if got != tc.want {
				t.Fatalf("NormalizePathSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestListSectionsAndTopicsFromRoot(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(t.TempDir(), "docs")
	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting started\n\nHello.\n")
	writeTestFile(t, filepath.Join(docsRoot, "reference", "query_language.md"), "# Query Language\n\nDetails.\n")
	writeTestFile(t, filepath.Join(docsRoot, "reference", "extra.md"), "# Extra\n")
	writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  guide:
    topics:
      getting-started:
        path: getting-started.md
  reference:
    topics:
      query-language:
        path: query_language.md
`)

	sections, err := ListSectionsFromRoot(docsRoot)
	if err != nil {
		t.Fatalf("ListSectionsFromRoot() error = %v", err)
	}
	var ids []string
	for _, section := range sections {
		ids = append(ids, section.ID)
	}
	if !slices.Equal(ids, []string{"guide", "reference"}) {
		t.Fatalf("section IDs = %v, want [guide reference]", ids)
	}

	topics, err := ListTopicsFromRoot(docsRoot, "reference")
	if err != nil {
		t.Fatalf("ListTopicsFromRoot() error = %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("expected 1 reference topic, got %d", len(topics))
	}
	if topics[0].ID != "query-language" || topics[0].Title != "Query Language" {
		t.Fatalf("topic = %#v, want query-language / Query Language", topics[0])
	}
}

func TestListSectionsAndTopicsWithIndexOverrides(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(t.TempDir(), "docs")
	writeTestFile(t, filepath.Join(docsRoot, "guide", "cli.md"), "# CLI Guide\n")
	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting Started\n")
	writeTestFile(t, filepath.Join(docsRoot, "reference", "query-language.md"), "# Query Language\n")
	writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  reference:
    title: Reference
    topics:
      query-language:
        path: query-language.md
  guide:
    title: User Guides
    topics:
      getting-started:
        title: Start Here
        path: getting-started.md
      cli:
        path: cli.md
`)

	sections, err := ListSectionsFromRoot(docsRoot)
	if err != nil {
		t.Fatalf("ListSectionsFromRoot() error = %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}
	if sections[0].ID != "reference" || sections[0].Title != "Reference" {
		t.Fatalf("first section = %#v, want reference / Reference", sections[0])
	}
	if sections[1].ID != "guide" || sections[1].Title != "User Guides" {
		t.Fatalf("second section = %#v, want guide / User Guides", sections[1])
	}

	topics, err := ListTopicsFromRoot(docsRoot, "guide")
	if err != nil {
		t.Fatalf("ListTopicsFromRoot() error = %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("expected 2 guide topics, got %d", len(topics))
	}
	if topics[0].ID != "getting-started" || topics[0].Title != "Start Here" || topics[1].ID != "cli" {
		t.Fatalf("topics = %#v", topics)
	}
}

func TestListSectionsFromRootFailures(t *testing.T) {
	t.Parallel()

	t.Run("without index", func(t *testing.T) {
		t.Parallel()

		docsRoot := filepath.Join(t.TempDir(), "docs")
		writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting Started\n")

		_, err := ListSectionsFromRoot(docsRoot)
		if err == nil {
			t.Fatal("expected ListSectionsFromRoot() to fail without docs index")
		}
		if !strings.Contains(err.Error(), "docs index not found") {
			t.Fatalf("error = %v, want missing docs index message", err)
		}
	})

	t.Run("missing indexed section", func(t *testing.T) {
		t.Parallel()

		docsRoot := filepath.Join(t.TempDir(), "docs")
		writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  missing:
    topics:
      intro:
        path: intro.md
`)

		_, err := ListSectionsFromRoot(docsRoot)
		if err == nil {
			t.Fatal("expected ListSectionsFromRoot() to fail for missing indexed section directory")
		}
		if !strings.Contains(err.Error(), `section "missing" not found`) {
			t.Fatalf("error = %v, want missing section message", err)
		}
	})
}

func TestListTopicsFromRootFailsForMissingIndexedFile(t *testing.T) {
	t.Parallel()

	docsRoot := filepath.Join(t.TempDir(), "docs")
	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting Started\n")
	writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  guide:
    topics:
      missing-topic:
        path: missing.md
`)

	_, err := ListTopicsFromRoot(docsRoot, "guide")
	if err == nil {
		t.Fatal("expected ListTopicsFromRoot() to fail for missing indexed topic file")
	}
	if !strings.Contains(err.Error(), `points to missing file "missing.md"`) {
		t.Fatalf("error = %v, want missing topic file message", err)
	}
}

func TestSearchFSPaginatesWithHasMore(t *testing.T) {
	docsFS := fstest.MapFS{
		"index.yaml": {
			Data: []byte(`sections:
  guide:
    topics:
      alpha:
        path: alpha.md
      beta:
        path: beta.md
`),
		},
		"guide/alpha.md": {Data: []byte("# Alpha\n\nquery one\nquery two\n")},
		"guide/beta.md":  {Data: []byte("# Beta\n\nquery three\nquery four\n")},
	}

	first, err := SearchFS(docsFS, ".", "query", "", 2, 0)
	if err != nil {
		t.Fatalf("SearchFS first page: %v", err)
	}
	if first.Returned != 2 || first.Limit != 2 || first.Offset != 0 {
		t.Fatalf("first page metadata = returned %d limit %d offset %d", first.Returned, first.Limit, first.Offset)
	}
	if !first.HasMore {
		t.Fatalf("first page HasMore = false, want true")
	}
	if got := first.Matches[0].Line; got != 3 {
		t.Fatalf("first match line = %d, want 3", got)
	}

	second, err := SearchFS(docsFS, ".", "query", "", 2, 2)
	if err != nil {
		t.Fatalf("SearchFS second page: %v", err)
	}
	if second.Returned != 2 || second.Limit != 2 || second.Offset != 2 {
		t.Fatalf("second page metadata = returned %d limit %d offset %d", second.Returned, second.Limit, second.Offset)
	}
	if second.HasMore {
		t.Fatalf("second page HasMore = true, want false")
	}
	if got := second.Matches[0].Topic; got != "beta" {
		t.Fatalf("second page first topic = %q, want beta", got)
	}
}

func TestSearchFSExactLimitDoesNotReportHasMore(t *testing.T) {
	docsFS := fstest.MapFS{
		"index.yaml": {
			Data: []byte(`sections:
  guide:
    topics:
      alpha:
        path: alpha.md
`),
		},
		"guide/alpha.md": {Data: []byte("# Alpha\n\nquery one\nquery two\n")},
	}

	result, err := SearchFS(docsFS, ".", "query", "", 2, 0)
	if err != nil {
		t.Fatalf("SearchFS: %v", err)
	}
	if result.HasMore {
		t.Fatalf("HasMore = true, want false")
	}
	if result.Returned != 2 {
		t.Fatalf("Returned = %d, want 2", result.Returned)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestSearchFSRejectsInvalidOffset(t *testing.T) {
	_, err := SearchFS(fstest.MapFS{}, ".", "query", "", 2, -1)
	if err == nil {
		t.Fatalf("SearchFS error = nil, want invalid offset error")
	}
}
