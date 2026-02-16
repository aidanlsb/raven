package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	builtindocs "github.com/aidanlsb/raven/docs"
)

func TestListDocsSectionsFSLoadsEmbeddedDocs(t *testing.T) {
	t.Parallel()

	sections, err := listDocsSectionsFS(builtindocs.FS, ".")
	if err != nil {
		t.Fatalf("listDocsSectionsFS() error = %v", err)
	}
	if len(sections) == 0 {
		t.Fatalf("expected embedded docs sections, got none")
	}

	var ids []string
	for _, s := range sections {
		ids = append(ids, s.ID)
	}
	for _, expected := range []string{"design", "guide", "reference"} {
		if !slices.Contains(ids, expected) {
			t.Fatalf("expected section %q in %v", expected, ids)
		}
	}
}

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

func TestListDocsSectionsAndTopics(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

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

	sections, err := listDocsSections(docsRoot)
	if err != nil {
		t.Fatalf("listDocsSections() error = %v", err)
	}

	var ids []string
	for _, s := range sections {
		ids = append(ids, s.ID)
	}
	if !slices.Equal(ids, []string{"guide", "reference"}) {
		t.Fatalf("section IDs = %v, want [guide reference]", ids)
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

func TestListDocsSectionsAndTopicsWithIndexOverrides(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

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

	sections, err := listDocsSections(docsRoot)
	if err != nil {
		t.Fatalf("listDocsSections() error = %v", err)
	}
	if len(sections) != 2 {
		t.Fatalf("expected 2 sections, got %d", len(sections))
	}

	if sections[0].ID != "reference" {
		t.Fatalf("first section ID = %q, want reference", sections[0].ID)
	}
	if sections[0].Title != "Reference" {
		t.Fatalf("first section title = %q, want Reference", sections[0].Title)
	}
	if sections[1].ID != "guide" {
		t.Fatalf("second section ID = %q, want guide", sections[1].ID)
	}
	if sections[1].Title != "User Guides" {
		t.Fatalf("second section title = %q, want User Guides", sections[1].Title)
	}

	topics, err := listDocsTopics(docsRoot, "guide")
	if err != nil {
		t.Fatalf("listDocsTopics() error = %v", err)
	}
	if len(topics) != 2 {
		t.Fatalf("expected 2 guide topics, got %d", len(topics))
	}
	if topics[0].ID != "getting-started" {
		t.Fatalf("first topic ID = %q, want getting-started", topics[0].ID)
	}
	if topics[0].Title != "Start Here" {
		t.Fatalf("first topic title = %q, want Start Here", topics[0].Title)
	}
	if topics[1].ID != "cli" {
		t.Fatalf("second topic ID = %q, want cli", topics[1].ID)
	}
}

func TestListDocsSectionsFailsWithoutIndex(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting Started\n")

	_, err := listDocsSections(docsRoot)
	if err == nil {
		t.Fatal("expected listDocsSections() to fail without docs index")
	}
	if !strings.Contains(err.Error(), "docs index not found") {
		t.Fatalf("error = %v, want missing docs index message", err)
	}
}

func TestListDocsSectionsFailsForMissingIndexedSection(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

	writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  missing:
    topics:
      intro:
        path: intro.md
`)

	_, err := listDocsSections(docsRoot)
	if err == nil {
		t.Fatal("expected listDocsSections() to fail for missing indexed section directory")
	}
	if !strings.Contains(err.Error(), `section "missing" not found`) {
		t.Fatalf("error = %v, want missing section message", err)
	}
}

func TestListDocsTopicsFailsForMissingIndexedFile(t *testing.T) {
	t.Parallel()

	tmp := t.TempDir()
	docsRoot := filepath.Join(tmp, "docs")

	writeTestFile(t, filepath.Join(docsRoot, "guide", "getting-started.md"), "# Getting Started\n")
	writeTestFile(t, filepath.Join(docsRoot, "index.yaml"), `sections:
  guide:
    topics:
      missing-topic:
        path: missing.md
`)

	_, err := listDocsTopics(docsRoot, "guide")
	if err == nil {
		t.Fatal("expected listDocsTopics() to fail for missing indexed topic file")
	}
	if !strings.Contains(err.Error(), `points to missing file "missing.md"`) {
		t.Fatalf("error = %v, want missing topic file message", err)
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

func TestOutputDocsSectionsTextListsSectionCommands(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	out := captureStdout(t, func() {
		err := outputDocsSections([]docsSectionView{
			{ID: "guide", Title: "User Guides", TopicCount: 5},
			{ID: "reference", Title: "Reference", TopicCount: 1},
		})
		if err != nil {
			t.Fatalf("outputDocsSections() error = %v", err)
		}
	})

	wantSnippets := []string{
		"Documentation section commands:",
		"rvn docs guide",
		"User Guides (5 topics)",
		"rvn docs reference",
		"Reference (1 topic)",
		"General docs commands:",
		"rvn docs list",
		"rvn docs <section>",
		"rvn docs <section> <topic>",
		"rvn docs search <query>",
		"rvn help <command>",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(out, snippet) {
			t.Fatalf("output missing %q\nfull output:\n%s", snippet, out)
		}
	}
}

func TestOutputDocsTopicsTextListsTopicCommands(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	section := docsSectionView{ID: "reference", Title: "Reference", TopicCount: 2}
	out := captureStdout(t, func() {
		err := outputDocsTopics(section, []docsTopicRecord{
			{Section: "reference", ID: "query-language", Title: "Query Language"},
			{Section: "reference", ID: "cli", Title: "CLI Reference"},
		})
		if err != nil {
			t.Fatalf("outputDocsTopics() error = %v", err)
		}
	})

	wantSnippets := []string{
		"Documentation topic commands for Reference [reference]:",
		"rvn docs reference query-language",
		"Query Language",
		"rvn docs reference cli",
		"CLI Reference",
		"General docs commands:",
		"rvn docs reference",
		"rvn docs search <query> --section reference",
		"rvn docs list",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(out, snippet) {
			t.Fatalf("output missing %q\nfull output:\n%s", snippet, out)
		}
	}
}

func TestOutputDocsTopicsTextHandlesEmptyTopicList(t *testing.T) {
	prevJSON := jsonOutput
	t.Cleanup(func() {
		jsonOutput = prevJSON
	})
	jsonOutput = false

	section := docsSectionView{ID: "design", Title: "Design Notes", TopicCount: 0}
	out := captureStdout(t, func() {
		err := outputDocsTopics(section, nil)
		if err != nil {
			t.Fatalf("outputDocsTopics() error = %v", err)
		}
	})

	wantSnippets := []string{
		"Documentation topic commands for Design Notes [design]:",
		"(no topics)",
		"General docs commands:",
		"rvn docs list",
		"rvn docs search <query> --section design",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(out, snippet) {
			t.Fatalf("output missing %q\nfull output:\n%s", snippet, out)
		}
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
