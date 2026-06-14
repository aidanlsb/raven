package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/picker"
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

func TestShouldUseDocsPickerNavigator(t *testing.T) {
	prevJSON := jsonOutput
	prevStdinTTY := interactiveStdinIsTerminal
	prevStdoutTTY := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		interactiveStdinIsTerminal = prevStdinTTY
		interactiveStdoutIsTerminal = prevStdoutTTY
	})

	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }

	jsonOutput = false
	if !shouldUseDocsPickerNavigator() {
		t.Fatalf("expected interactive docs mode when TTY is available")
	}

	jsonOutput = true
	if shouldUseDocsPickerNavigator() {
		t.Fatalf("expected interactive docs mode to be disabled for --json")
	}

	jsonOutput = false
	interactiveStdinIsTerminal = func() bool { return false }
	if shouldUseDocsPickerNavigator() {
		t.Fatalf("expected interactive docs mode to be disabled when stdin is not a TTY")
	}
}

func TestPickDocsSection(t *testing.T) {
	prevRun := docsRunPicker
	t.Cleanup(func() {
		docsRunPicker = prevRun
	})

	sections := []docsSectionView{
		{ID: "guide", Title: "User Guides", TopicCount: 5},
		{ID: "reference", Title: "Reference", TopicCount: 9},
	}

	docsRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if opts.Prompt != "docs/section" {
			t.Fatalf("prompt = %q, want docs/section", opts.Prompt)
		}
		if opts.Title != "Select a docs section" {
			t.Fatalf("title = %q, want Select a docs section", opts.Title)
		}
		if !opts.AllowForward {
			t.Fatalf("expected section picker to allow forward navigation")
		}
		if !hasShortcutTip(opts.Shortcuts, "l", "topics") {
			t.Fatalf("expected section picker shortcut tips, got %#v", opts.Shortcuts)
		}
		if len(items) != 2 {
			t.Fatalf("expected 2 items, got %d", len(items))
		}
		return picker.Selection{Item: items[1]}, true, nil
	}

	selected, ok, err := pickDocsSection(sections)
	if err != nil {
		t.Fatalf("pickDocsSection() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected section to be selected")
	}
	if selected.ID != "reference" {
		t.Fatalf("selected section ID = %q, want reference", selected.ID)
	}
}

func TestPickDocsTopicCancelled(t *testing.T) {
	prevRun := docsRunPicker
	t.Cleanup(func() {
		docsRunPicker = prevRun
	})

	section := docsSectionView{ID: "reference", Title: "Reference", TopicCount: 1}
	topics := []docsTopicRecord{
		{Section: "reference", ID: "query-language", Title: "Query Language"},
	}

	docsRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if len(items) != 1 {
			t.Fatalf("expected 1 topic item, got %d", len(items))
		}
		if opts.Prompt != "docs/reference" {
			t.Fatalf("unexpected prompt: %q", opts.Prompt)
		}
		return picker.Selection{}, false, nil
	}

	_, _, ok, err := pickDocsTopic(section, topics)
	if err != nil {
		t.Fatalf("pickDocsTopic() error = %v", err)
	}
	if ok {
		t.Fatalf("expected cancelled selection to return ok=false")
	}
}

func TestPickDocsTopicBackAction(t *testing.T) {
	prevRun := docsRunPicker
	t.Cleanup(func() {
		docsRunPicker = prevRun
	})

	section := docsSectionView{ID: "reference", Title: "Reference", TopicCount: 1}
	topics := []docsTopicRecord{
		{Section: "reference", ID: "query-language", Title: "Query Language"},
	}

	docsRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if !opts.AllowBack || !opts.AllowForward {
			t.Fatalf("expected topic picker to allow back and forward navigation")
		}
		if !hasShortcutTip(opts.Shortcuts, "h", "sections") || !hasShortcutTip(opts.Shortcuts, "l", "open") {
			t.Fatalf("expected topic picker shortcut tips, got %#v", opts.Shortcuts)
		}
		return picker.Selection{Action: picker.ActionBack}, true, nil
	}

	_, action, ok, err := pickDocsTopic(section, topics)
	if err != nil {
		t.Fatalf("pickDocsTopic() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected back action to return ok=true")
	}
	if action != picker.ActionBack {
		t.Fatalf("action = %q, want back", action)
	}
}

func hasShortcutTip(shortcuts []picker.ShortcutTip, key, description string) bool {
	for _, shortcut := range shortcuts {
		if shortcut.Key == key && shortcut.Description == description {
			return true
		}
	}
	return false
}

func TestRunDocsPickerNavigatorCanGoBackToSections(t *testing.T) {
	prevRun := docsRunPicker
	prevJSON := jsonOutput
	prevDisplay := docsDisplayContext
	prevRender := docsMarkdownRender
	t.Cleanup(func() {
		docsRunPicker = prevRun
		jsonOutput = prevJSON
		docsDisplayContext = prevDisplay
		docsMarkdownRender = prevRender
	})

	jsonOutput = false
	docsDisplayContext = func() *ui.DisplayContext {
		return &ui.DisplayContext{TermWidth: 100, IsTTY: true}
	}
	docsMarkdownRender = func(content string, _ int) (string, error) {
		if !strings.Contains(content, "# Query Language") {
			t.Fatalf("expected final selected topic to be query language")
		}
		return "RENDERED QUERY LANGUAGE\n", nil
	}

	step := 0
	docsRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		step++
		switch step {
		case 1:
			if opts.Prompt != "docs/section" {
				t.Fatalf("step 1 prompt = %q", opts.Prompt)
			}
			return picker.Selection{Item: picker.Item{ID: "getting-started"}, Action: picker.ActionForward}, true, nil
		case 2:
			if opts.Prompt != "docs/getting-started" {
				t.Fatalf("step 2 prompt = %q", opts.Prompt)
			}
			return picker.Selection{Action: picker.ActionBack}, true, nil
		case 3:
			if opts.Prompt != "docs/section" {
				t.Fatalf("step 3 prompt = %q", opts.Prompt)
			}
			return picker.Selection{Item: picker.Item{ID: "querying"}, Action: picker.ActionForward}, true, nil
		case 4:
			if opts.Prompt != "docs/querying" {
				t.Fatalf("step 4 prompt = %q", opts.Prompt)
			}
			for _, item := range items {
				if item.ID == "query-language" {
					return picker.Selection{Item: item, Action: picker.ActionForward}, true, nil
				}
			}
			t.Fatalf("query-language topic missing from %#v", items)
		default:
			t.Fatalf("unexpected picker step %d", step)
		}
		return picker.Selection{}, false, nil
	}

	docsFS := os.DirFS(filepath.Join(repoRoot(t), "docs"))
	out := captureStdout(t, func() {
		err := runDocsPickerNavigator(docsFS, []docsSectionView{
			{ID: "getting-started", Title: "Getting Started", TopicCount: 1},
			{ID: "querying", Title: "Querying", TopicCount: 1},
		})
		if err != nil {
			t.Fatalf("runDocsPickerNavigator() error = %v", err)
		}
	})

	if step != 4 {
		t.Fatalf("picker steps = %d, want 4", step)
	}
	if !strings.Contains(out, "RENDERED QUERY LANGUAGE") {
		t.Fatalf("output missing rendered topic:\n%s", out)
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
