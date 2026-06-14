package ui

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestTruncateWithEllipsisPreservesUTF8(t *testing.T) {
	t.Parallel()

	got := TruncateWithEllipsis("ééééé", 4)
	if got != "é..." {
		t.Fatalf("expected UTF-8-safe truncation, got %q", got)
	}
	if !utf8.ValidString(got) {
		t.Fatalf("expected valid UTF-8 output, got %q", got)
	}
}

func TestWrapTextTwoLinesPreservesUTF8(t *testing.T) {
	t.Parallel()

	line1, line2 := WrapTextTwoLines("éééé éééé", 4)
	if line1 != "éééé" {
		t.Fatalf("expected first line to preserve multibyte characters, got %q", line1)
	}
	if line2 != "éééé" {
		t.Fatalf("expected second line to preserve multibyte characters, got %q", line2)
	}
}

func TestVisibleLenHandlesUnicodeAndANSI(t *testing.T) {
	t.Parallel()

	if got := VisibleLen("\x1b[31m猫\x1b[0m"); got != 2 {
		t.Fatalf("expected visible width 2 for colored CJK rune, got %d", got)
	}
}

func TestBacklinksLayoutStylesPrimaryContentLikeQueryResults(t *testing.T) {
	t.Parallel()

	columns := BacklinksLayout()
	if len(columns) != 3 {
		t.Fatalf("expected 3 columns, got %d", len(columns))
	}
	if columns[1].Name != "content" {
		t.Fatalf("expected primary backlinks column to be content, got %q", columns[1].Name)
	}
	if columns[1].HasStyle {
		t.Fatalf("expected primary backlinks content to use default styling")
	}
	if columns[2].Name != "file" || !columns[2].HasStyle {
		t.Fatalf("expected backlinks location column to stay styled as file metadata")
	}
}

func TestObjectLayoutPrioritizesNameColumn(t *testing.T) {
	t.Parallel()

	columns := ObjectLayout([]string{"category", "project", "status"})
	headers := []string{"#", "title", "category", "project", "status", "location"}
	rows := [][]string{
		{
			"1",
			"Consider an interactive option for queries that opens fzf",
			"-",
			"raven",
			"open",
			"type/issue/consider-an-interactive-option-for-queries-that-opens-fzf.md:1",
		},
		{
			"2",
			"Improve stale index schema error handling for query",
			"suggestion",
			"raven",
			"open",
			"type/issue/improve-stale-index-schema-error-handling-for-query.md:1",
		},
	}
	widths := CalculateColumnWidthsForRows(columns, headers, rows, 120)
	if len(widths) != 6 {
		t.Fatalf("expected 6 columns, got %d", len(widths))
	}

	nameWidth := widths[1]
	locationWidth := widths[len(widths)-1]
	if nameWidth < 50 {
		t.Fatalf("name width = %d, want at least 50", nameWidth)
	}
	if locationWidth >= nameWidth {
		t.Fatalf("location width = %d, want less than name width %d", locationWidth, nameWidth)
	}
	if widths[2] != len("suggestion") {
		t.Fatalf("category width = %d, want to fit suggestion", widths[2])
	}
	if widths[3] != len("project") {
		t.Fatalf("project width = %d, want to fit header", widths[3])
	}
	if widths[4] != len("status") {
		t.Fatalf("status width = %d, want to fit header", widths[4])
	}

	total := 0
	for _, width := range widths {
		total += width
	}
	total += 2 * (len(widths) - 1)
	if total > 120 {
		t.Fatalf("table width = %d, want <= 120 (widths=%v)", total, widths)
	}
}

func TestResultsTableTreatsColumnWidthsAsContentWidths(t *testing.T) {
	t.Parallel()

	table := NewResultsTable(&DisplayContext{TermWidth: 120}, ObjectLayout([]string{"category", "project", "status"}))
	table.SetHeaders([]string{"#", "title", "category", "project", "status", "location"})
	table.AddRow(ResultRow{
		Num: 1,
		Cells: []string{
			" 1",
			"Improve stale index schema error handling for query",
			"sugge...",
			"raven",
			"open",
			"type/issue/improve-stale-index-schema-error-handling-for-query.md:1",
		},
	})

	out := table.Render()
	if strings.Contains(out, "catego\n") || strings.Contains(out, "sugge.\n") {
		t.Fatalf("expected compact columns to truncate, not wrap:\n%s", out)
	}
	if !strings.Contains(out, "category") {
		t.Fatalf("expected full category header, got:\n%s", out)
	}
}
