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
	items := result.DataList("items")
	if len(items) < 2 {
		t.Errorf("expected at least 2 search results, got %d", len(items))
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

func TestIntegration_AddToSectionUsesDirectBodyRange(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("project.md", `---
type: page
---
# Project

### Bugs / Fixes
- Existing item

#### Child
- Child item

### Other
- Keep this below
`).
		Build()
	v.RunCLI("reindex").MustSucceed(t)

	result := v.RunCLI("add", "Another bug item", "--to", "project#bugs-fixes")
	result.MustSucceed(t)

	lineValue, ok := result.Data["line"].(float64)
	if !ok {
		t.Fatalf("expected numeric line in result data, got %#v", result.Data["line"])
	}
	if int(lineValue) != 8 {
		t.Fatalf("line = %v, want 8", lineValue)
	}
	content, err := os.ReadFile(filepath.Join(v.Path, "project.md"))
	if err != nil {
		t.Fatalf("read project.md: %v", err)
	}
	text := string(content)
	if !strings.Contains(text, "Another bug item") {
		t.Fatalf("expected new section content in project.md, got:\n%s", text)
	}
	if strings.Index(text, "Another bug item") > strings.Index(text, "#### Child") {
		t.Fatalf("expected direct-body append before child subtree, got:\n%s", text)
	}
}

func TestIntegration_AddRejectsRemovedHeadingFlags(t *testing.T) {
	t.Parallel()

	for _, flagArgs := range [][]string{
		{"--heading", "### Log"},
		{"--create-heading"},
	} {
		flagArgs := flagArgs
		t.Run(flagArgs[0], func(t *testing.T) {
			t.Parallel()
			v := testutil.NewTestVault(t).
				WithSchema(testutil.MinimalSchema()).
				WithFile("project.md", "---\ntype: page\n---\n# Project\n").
				Build()

			args := []string{"add", "New item", "--to", "project.md"}
			args = append(args, flagArgs...)
			result := v.RunCLI(args...)
			result.MustFail(t, "INVALID_INPUT")
			result.MustFailWithMessage(t, "rvn section create")
			if strings.Contains(v.ReadFile("project.md"), "New item") {
				t.Fatalf("rejected add changed file:\n%s", v.ReadFile("project.md"))
			}
		})
	}
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
