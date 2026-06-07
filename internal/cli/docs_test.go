package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/ui"
)

func repoRoot(t *testing.T) string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
}

func TestResolveCLICommandPath(t *testing.T) {
	t.Parallel()

	if got, ok := resolveCLICommandPath([]string{"query"}); !ok || got != "query" {
		t.Fatalf("resolveCLICommandPath(query) = (%q, %v), want (query, true)", got, ok)
	}

	if got, ok := resolveCLICommandPath([]string{"schema", "add", "type"}); !ok || got != "schema add type" {
		t.Fatalf("resolveCLICommandPath(schema add type) = (%q, %v), want (schema add type, true)", got, ok)
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
			{ID: "getting-started", Title: "Getting Started", TopicCount: 3},
			{ID: "querying", Title: "Querying", TopicCount: 1},
		})
		if err != nil {
			t.Fatalf("outputDocsSections() error = %v", err)
		}
	})

	wantSnippets := []string{
		"Documentation section commands",
		"rvn docs getting-started",
		"Getting Started (3 topics)",
		"rvn docs querying",
		"Querying (1 topic)",
		"General docs commands",
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
		"Documentation topic commands for Reference [reference]",
		"rvn docs reference query-language",
		"Query Language",
		"rvn docs reference cli",
		"CLI Reference",
		"General docs commands",
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

	section := docsSectionView{ID: "vault-management", Title: "Vault Management", TopicCount: 0}
	out := captureStdout(t, func() {
		err := outputDocsTopics(section, nil)
		if err != nil {
			t.Fatalf("outputDocsTopics() error = %v", err)
		}
	})

	wantSnippets := []string{
		"Documentation topic commands for Vault Management [vault-management]",
		"(no topics)",
		"General docs commands",
		"rvn docs list",
		"rvn docs search <query> --section vault-management",
	}
	for _, snippet := range wantSnippets {
		if !strings.Contains(out, snippet) {
			t.Fatalf("output missing %q\nfull output:\n%s", snippet, out)
		}
	}
}

func TestShouldUseDocsFZFNavigator(t *testing.T) {
	prevJSON := jsonOutput
	prevLookPath := fzfLookPath
	prevStdinTTY := fzfStdinIsTerminal
	prevStdoutTTY := fzfStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		fzfLookPath = prevLookPath
		fzfStdinIsTerminal = prevStdinTTY
		fzfStdoutIsTerminal = prevStdoutTTY
	})

	fzfStdinIsTerminal = func() bool { return true }
	fzfStdoutIsTerminal = func() bool { return true }
	fzfLookPath = func(file string) (string, error) {
		if file == "fzf" {
			return "/usr/local/bin/fzf", nil
		}
		return "", exec.ErrNotFound
	}

	jsonOutput = false
	if !shouldUseDocsFZFNavigator() {
		t.Fatalf("expected interactive docs mode when TTY and fzf is available")
	}

	jsonOutput = true
	if shouldUseDocsFZFNavigator() {
		t.Fatalf("expected interactive docs mode to be disabled for --json")
	}

	jsonOutput = false
	fzfStdinIsTerminal = func() bool { return false }
	if shouldUseDocsFZFNavigator() {
		t.Fatalf("expected interactive docs mode to be disabled when stdin is not a TTY")
	}

	fzfStdinIsTerminal = func() bool { return true }
	fzfLookPath = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	if shouldUseDocsFZFNavigator() {
		t.Fatalf("expected interactive docs mode to be disabled when fzf is unavailable")
	}
}

func TestPickDocsSectionWithFZF(t *testing.T) {
	prevRun := docsFZFRun
	t.Cleanup(func() {
		docsFZFRun = prevRun
	})

	sections := []docsSectionView{
		{ID: "guide", Title: "User Guides", TopicCount: 5},
		{ID: "reference", Title: "Reference", TopicCount: 9},
	}

	docsFZFRun = func(lines []string, prompt, header string) (string, bool, error) {
		if prompt != "docs/section> " {
			t.Fatalf("prompt = %q, want docs/section> ", prompt)
		}
		if !strings.Contains(header, "Select a docs section") {
			t.Fatalf("unexpected header %q", header)
		}
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines, got %d", len(lines))
		}
		return lines[1], true, nil
	}

	selected, ok, err := pickDocsSectionWithFZF(sections)
	if err != nil {
		t.Fatalf("pickDocsSectionWithFZF() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected section to be selected")
	}
	if selected.ID != "reference" {
		t.Fatalf("selected section ID = %q, want reference", selected.ID)
	}
}

func TestPickDocsTopicWithFZFCancelled(t *testing.T) {
	prevRun := docsFZFRun
	t.Cleanup(func() {
		docsFZFRun = prevRun
	})

	section := docsSectionView{ID: "reference", Title: "Reference", TopicCount: 1}
	topics := []docsTopicRecord{
		{Section: "reference", ID: "query-language", Title: "Query Language"},
	}

	docsFZFRun = func(lines []string, prompt, header string) (string, bool, error) {
		if len(lines) != 1 {
			t.Fatalf("expected 1 topic line, got %d", len(lines))
		}
		if !strings.Contains(prompt, "docs/reference> ") {
			t.Fatalf("unexpected prompt: %q", prompt)
		}
		return "", false, nil
	}

	_, ok, err := pickDocsTopicWithFZF(section, topics)
	if err != nil {
		t.Fatalf("pickDocsTopicWithFZF() error = %v", err)
	}
	if ok {
		t.Fatalf("expected cancelled selection to return ok=false")
	}
}

func TestDocsFZFSelectionID(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "id with title", in: "reference\tReference", want: "reference"},
		{name: "id only", in: "query-language", want: "query-language"},
		{name: "trim whitespace", in: "  guide\tUser Guides  ", want: "guide"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := docsFZFSelectionID(tc.in)
			if got != tc.want {
				t.Fatalf("docsFZFSelectionID(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestOutputDocsTopicContentRendersMarkdownInTTY(t *testing.T) {
	prevJSON := jsonOutput
	prevDisplay := docsDisplayContext
	prevRender := docsMarkdownRender
	t.Cleanup(func() {
		jsonOutput = prevJSON
		docsDisplayContext = prevDisplay
		docsMarkdownRender = prevRender
	})

	jsonOutput = false
	docsDisplayContext = func() *ui.DisplayContext {
		return &ui.DisplayContext{TermWidth: 100, IsTTY: true}
	}
	docsMarkdownRender = func(content string, width int) (string, error) {
		if width != 100 {
			t.Fatalf("render width = %d, want 100", width)
		}
		if !strings.Contains(content, "# Query Language") {
			t.Fatalf("expected topic markdown content to be passed to renderer")
		}
		return "RENDERED TOPIC\n", nil
	}

	docsFS := os.DirFS(filepath.Join(repoRoot(t), "docs"))
	out := captureStdout(t, func() {
		err := outputDocsTopicContent(docsFS, docsTopicRecord{
			Section: "querying",
			ID:      "query-language",
			Title:   "Query Language",
			Path:    "docs/querying/query-language.md",
			FSPath:  "querying/query-language.md",
		})
		if err != nil {
			t.Fatalf("outputDocsTopicContent() error = %v", err)
		}
	})

	if !strings.Contains(out, "Path: docs/querying/query-language.md") {
		t.Fatalf("expected output path header, got:\n%s", out)
	}
	if !strings.Contains(out, "RENDERED TOPIC") {
		t.Fatalf("expected rendered markdown output, got:\n%s", out)
	}
}

func TestOutputDocsTopicContentSkipsRendererWhenNotTTY(t *testing.T) {
	prevJSON := jsonOutput
	prevDisplay := docsDisplayContext
	prevRender := docsMarkdownRender
	t.Cleanup(func() {
		jsonOutput = prevJSON
		docsDisplayContext = prevDisplay
		docsMarkdownRender = prevRender
	})

	jsonOutput = false
	docsDisplayContext = func() *ui.DisplayContext {
		return &ui.DisplayContext{TermWidth: 100, IsTTY: false}
	}
	docsMarkdownRender = func(string, int) (string, error) {
		t.Fatalf("renderer should not be called when output is not a TTY")
		return "", nil
	}

	docsFS := os.DirFS(filepath.Join(repoRoot(t), "docs"))
	out := captureStdout(t, func() {
		err := outputDocsTopicContent(docsFS, docsTopicRecord{
			Section: "querying",
			ID:      "query-language",
			Title:   "Query Language",
			Path:    "docs/querying/query-language.md",
			FSPath:  "querying/query-language.md",
		})
		if err != nil {
			t.Fatalf("outputDocsTopicContent() error = %v", err)
		}
	})

	if !strings.Contains(out, "# Query Language") {
		t.Fatalf("expected raw markdown output when not a TTY, got:\n%s", out)
	}
}
