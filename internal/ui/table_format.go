package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// tableColumnGap is the spacing rendered between adjacent table columns. It is
// the visual counterpart to the inter-column padding reserved by the width
// calculators (see CalculateColumnWidths), so the interactive picker and the
// printed result tables lay columns out identically.
const tableColumnGap = "  "

// TableRowStyle selects the role a rendered row plays so shared cell styling
// stays identical between the interactive picker and printed result tables.
type TableRowStyle struct {
	// Header renders the row as a bold, muted header instead of a data row.
	Header bool
	// Selected bolds a data row to mark the picker cursor/selection. It is a
	// no-op for header rows.
	Selected bool
}

// FormatTableRow renders a single table row from pre-measured column widths.
// Each cell is truncated to its column width, aligned and styled per its
// ColumnDef, and the cells are joined with the shared inter-column gap.
//
// Both the interactive picker and ResultsTable render through this helper so
// column widths, alignment, per-column styles, and header/selection emphasis
// cannot silently diverge between printed and interactive tables. Pass nil for
// renderer to use the default lipgloss renderer.
func FormatTableRow(renderer *lipgloss.Renderer, cells []string, widths []int, columns []ColumnDef, style TableRowStyle) string {
	renderer = rendererOrDefault(renderer)
	parts := make([]string, 0, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = truncateCell(cell, width)
		cellStyle := TableCellStyle(renderer, width, columns, i, style)
		parts = append(parts, cellStyle.Render(cell))
	}
	return strings.Join(parts, tableColumnGap)
}

// TableCellStyle returns the lipgloss style for a single table cell at the
// given column index. It applies the column's alignment, promotes header cells
// to muted bold, applies the column's configured style to data cells, and bolds
// selected data rows. Pass nil for renderer to use the default lipgloss
// renderer.
func TableCellStyle(renderer *lipgloss.Renderer, width int, columns []ColumnDef, index int, style TableRowStyle) lipgloss.Style {
	renderer = rendererOrDefault(renderer)
	cellStyle := renderer.NewStyle().Width(width)
	if index < len(columns) {
		switch columns[index].Align {
		case AlignRight:
			cellStyle = cellStyle.Align(lipgloss.Right)
		case AlignCenter:
			cellStyle = cellStyle.Align(lipgloss.Center)
		default:
			cellStyle = cellStyle.Align(lipgloss.Left)
		}
		if style.Header {
			cellStyle = mutedStyleFor(renderer).Bold(true).Width(width)
		} else if columns[index].HasStyle {
			cellStyle = columns[index].Style.Renderer(renderer).Width(width)
		}
	}
	if style.Selected && !style.Header {
		cellStyle = cellStyle.Bold(true)
	}
	return cellStyle
}

// TableHeaderDivider renders the muted horizontal rule drawn beneath a table's
// header row. Pass nil for renderer to use the default lipgloss renderer.
func TableHeaderDivider(renderer *lipgloss.Renderer, width int) string {
	renderer = rendererOrDefault(renderer)
	if width < 1 {
		width = 1
	}
	return mutedStyleFor(renderer).Render(strings.Repeat("─", width))
}

// TableRowWidth reports the rendered width of a row with the given column
// widths, including the inter-column gaps FormatTableRow inserts. It lets
// callers size a header divider to match the row content.
func TableRowWidth(widths []int) int {
	if len(widths) == 0 {
		return 0
	}
	total := 0
	for _, width := range widths {
		total += width
	}
	total += len(tableColumnGap) * (len(widths) - 1)
	return total
}

func mutedStyleFor(renderer *lipgloss.Renderer) lipgloss.Style {
	return rendererOrDefault(renderer).NewStyle().Foreground(lipgloss.Color("8"))
}

func rendererOrDefault(renderer *lipgloss.Renderer) *lipgloss.Renderer {
	if renderer != nil {
		return renderer
	}
	return lipgloss.DefaultRenderer()
}
