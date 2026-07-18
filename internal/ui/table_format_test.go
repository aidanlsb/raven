package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func TestFormatTableRowTruncatesAndJoinsWithColumnGap(t *testing.T) {
	t.Parallel()

	columns := []ColumnDef{
		{Name: "num", MinWidth: 3, Align: AlignRight, HasStyle: true, Style: Muted},
		{Name: "content", MinWidth: 4, Align: AlignLeft},
	}
	widths := []int{3, 4}

	row := FormatTableRow(nil, []string{"1", "hello world"}, widths, columns, TableRowStyle{})
	if !strings.Contains(row, tableColumnGap) {
		t.Fatalf("row should join columns with the shared gap: %q", row)
	}
	// The content cell is width 4, so "hello world" is truncated with an ellipsis.
	if !strings.Contains(row, "h...") {
		t.Fatalf("expected content cell truncated to width, got %q", row)
	}
	if width := ansi.StringWidth(row); width != TableRowWidth(widths) {
		t.Fatalf("row width = %d, want %d", width, TableRowWidth(widths))
	}
}

func TestTableCellStyleAppliesHeaderAndSelectionEmphasis(t *testing.T) {
	t.Parallel()

	columns := []ColumnDef{
		{Name: "num", MinWidth: 3, Align: AlignRight, HasStyle: true, Style: Muted},
		{Name: "content", MinWidth: 4, Align: AlignLeft},
	}

	if !TableCellStyle(nil, 10, columns, 1, TableRowStyle{Header: true}).GetBold() {
		t.Fatalf("header cell should be bold")
	}
	if !TableCellStyle(nil, 10, columns, 1, TableRowStyle{Selected: true}).GetBold() {
		t.Fatalf("selected data cell should be bold")
	}
	if TableCellStyle(nil, 10, columns, 1, TableRowStyle{}).GetBold() {
		t.Fatalf("plain data cell should not be bold")
	}
	// Selection must not bold a header row.
	if !TableCellStyle(nil, 10, columns, 1, TableRowStyle{Header: true, Selected: true}).GetBold() {
		t.Fatalf("header remains bold regardless of selection")
	}
}

func TestTableHeaderDividerMatchesRowWidth(t *testing.T) {
	t.Parallel()

	widths := []int{3, 10, 6}
	divider := TableHeaderDivider(nil, TableRowWidth(widths))
	if got, want := ansi.StringWidth(divider), TableRowWidth(widths); got != want {
		t.Fatalf("divider width = %d, want %d", got, want)
	}
	if strings.Trim(ansi.Strip(divider), "─") != "" {
		t.Fatalf("divider should only contain box-drawing rule characters: %q", divider)
	}
}
