package cli

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestNormalizeDocsPathSlug(t *testing.T) {
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
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeDocsPathSlug(tc.in)
			if got != tc.want {
				t.Fatalf("normalizeDocsPathSlug(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestListDocsCategoriesAndTopics(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting started\n\nHello.\n")
	writeTestFile(t, filepath.Join(docsRoot, "reference", "query_language.md"), "# Query Language\n\nDetails.\n")
	writeTestFile(t, filepath.Join(docsRoot, "reference", "_draft.md"), "# Draft\n")
	writeTestFile(t, filepath.Join(docsRoot, "_internal", "notes.md"), "# Notes\n")

	categories, err := listDocsCategories(docsRoot)
	if err != nil {
		t.Fatalf("listDocsCategories() error = %v", err)
	}

	var ids []string
	for _, c := range categories {
		ids = append(ids, c.ID)
	}
	if !slices.Equal(ids, []string{"guide", "reference"}) {
		t.Fatalf("category IDs = %v, want [guide reference]", ids)
	}

	topics, err := listDocsTopics(docsRoot, "reference")
	if err != nil {
		t.Fatalf("listDocsTopics() error = %v", err)
	}
	if len(topics) != 1 {
		t.Fatalf("expected 1 reference topic, got %d", len(topics))
	}
	if topics[0].ID != "query-language" {
		t.Fatalf("topic ID = %q, want query-language", topics[0].ID)
	}
	if topics[0].Title != "Query Language" {
		t.Fatalf("topic Title = %q, want Query Language", topics[0].Title)
	}
}

func TestResolveCLICommandPath(t *testing.T) {
	t.Parallel()

	if got, ok := resolveCLICommandPath([]string{"query"}); !ok || got != "query" {
		t.Fatalf("resolveCLICommandPath(query) = (%q, %v), want (query, true)", got, ok)
	}

	if got, ok := resolveCLICommandPath([]string{"schema", "add", "type"}); !ok || got != "schema add" {
		t.Fatalf("resolveCLICommandPath(schema add type) = (%q, %v), want (schema add, true)", got, ok)
	}

	if _, ok := resolveCLICommandPath([]string{"not-a-command"}); ok {
		t.Fatalf("expected unknown command path to return ok=false")
	}
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
