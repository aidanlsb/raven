//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

// TestIntegration_Search tests full-text search.
func TestIntegration_Search(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/meeting.md", `---
type: page
---
# Team Meeting Notes

Discussed the quarterly roadmap and budget allocation.
`).
		WithFile("notes/todo.md", `---
type: page
---
# Todo List

- Review quarterly report
- Prepare presentation
`).
		Build()

	// Reindex
	v.RunCLI("reindex").MustSucceed(t)

	// Search for quarterly
	result := v.RunCLI("search", "quarterly")
	result.MustSucceed(t)

	// Should find both files (may return more than 2 results because section
	// objects are also indexed — e.g. "# Team Meeting Notes" produces both a
	// page-level and a section-level FTS entry).
	results := result.DataList("results")
	if len(results) < 2 {
		t.Errorf("expected at least 2 search results, got %d", len(results))
	}
}

// TestIntegration_DailyNote tests daily note creation and management.
func TestIntegration_DailyNote(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(`version: 1
daily:
  directory: daily/
  template: |
    # Daily Note
    
    ## Tasks
`).
		Build()

	// The daily command may output human-readable text or JSON
	// Just verify that running it creates the daily directory
	_ = v.RunCLI("daily")

	// Verify the daily directory exists
	v.AssertDirExists("daily")
}

// TestIntegration_Add tests adding content to files.
func TestIntegration_Add(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("inbox.md", `---
type: page
---
# Inbox
`).
		Build()

	// Add content to inbox
	result := v.RunCLI("add", "New task for today", "--to", "inbox.md")
	result.MustSucceed(t)

	v.AssertFileContains("inbox.md", "New task for today")
}

func TestIntegration_AddToSectionBySlug(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item

### Other
- Keep this below
`).
		Build()

	result := v.RunCLI("add", "New bug item", "--to", "project.md", "--heading", "bugs-fixes")
	result.MustSucceed(t)

	content, err := os.ReadFile(filepath.Join(v.Path, "project.md"))
	if err != nil {
		t.Fatalf("read project.md: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "New bug item") {
		t.Fatalf("expected new section content in project.md, got:\n%s", text)
	}
	if strings.Index(text, "New bug item") > strings.Index(text, "### Other") {
		t.Fatalf("expected section append before next heading, got:\n%s", text)
	}
}

func TestIntegration_AddToSectionByHeadingText(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item
`).
		Build()

	result := v.RunCLI("add", "Another bug item", "--to", "project.md", "--heading", "### Bugs / Fixes")
	result.MustSucceed(t)
	v.AssertFileContains("project.md", "Another bug item")
}

func TestIntegration_AddToSectionBySingleWordHeadingText(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

## Description
Existing text
`).
		Build()

	result := v.RunCLI("add", "More detail", "--to", "project.md", "--heading", "Description")
	result.MustSucceed(t)
	v.AssertFileContains("project.md", "More detail")
}

func TestIntegration_AddToSectionReportsInsertedLine(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item

### Other
- Keep this below
`).
		Build()

	result := v.RunCLI("add", "Another bug item", "--to", "project.md", "--heading", "### Bugs / Fixes")
	result.MustSucceed(t)

	lineValue, ok := result.Data["line"].(float64)
	if !ok {
		t.Fatalf("expected numeric line in result data, got %#v", result.Data["line"])
	}
	if int(lineValue) != 8 {
		t.Fatalf("line = %v, want 8", lineValue)
	}
}

func TestIntegration_AddReportsStableHeadingErrorCodes(t *testing.T) {
	t.Parallel()

	t.Run("missing heading returns ref not found", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("project.md", `---
type: page
---
# Project

### Existing Heading
- Existing item
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "project.md", "--heading", "### Missing Heading")
		result.MustFail(t, "REF_NOT_FOUND")
	})

	t.Run("ambiguous heading text returns ref ambiguous", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("project.md", `---
type: page
---
# Project

### Team Notes
First section

### Team Notes
Second section
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "project.md", "--heading", "### Team Notes")
		result.MustFail(t, "REF_AMBIGUOUS")
	})

	t.Run("heading parse failure returns invalid input", func(t *testing.T) {
		v := testutil.NewTestVault(t).
			WithSchema(testutil.MinimalSchema()).
			WithFile("broken.md", `---
type: page
meta:
  nested: true
---
# Broken
`).
			Build()

		result := v.RunCLI("add", "New item", "--to", "broken.md", "--heading", "### Broken")
		result.MustFail(t, "INVALID_INPUT")
	})
}

func TestIntegration_ResolveAndAddPreferDynamicTodayOverSectionShortName(t *testing.T) {
	t.Parallel()
	today := time.Now().Format("2006-01-02")
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("daily/2026-03-16.md", `# Archive

# today
Old note
`).
		WithFile("daily/"+today+".md", `# Today
Current note
`).
		Build()

	v.RunCLI("reindex").MustSucceed(t)

	resolveResult := v.RunCLI("resolve", "today")
	resolveResult.MustSucceed(t)
	if resolveResult.DataString("object_id") != today {
		t.Fatalf("resolve object_id = %q, want %q", resolveResult.DataString("object_id"), today)
	}

	addResult := v.RunCLI("add", "New task for today", "--to", "today")
	addResult.MustSucceed(t)
	currentDaily := v.ReadFile("daily/" + today + ".md")
	if !strings.Contains(currentDaily, "New task for today") {
		t.Fatalf("expected current daily note to contain capture, got:\n%s", currentDaily)
	}
	archivedDaily := v.ReadFile("daily/2026-03-16.md")
	if strings.Contains(archivedDaily, "New task for today") {
		t.Fatalf("expected archived daily note to remain unchanged, got:\n%s", archivedDaily)
	}
}
