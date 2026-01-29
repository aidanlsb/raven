package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
)

// Alignment represents column text alignment.
type Alignment int

const (
	AlignLeft Alignment = iota
	AlignRight
	AlignCenter
)

// ColumnDef defines a column in a ResultsTable.
type ColumnDef struct {
	Name       string         // Header name (used for debugging, not displayed in minimal style)
	WidthRatio float64        // Proportion of available width (0.0-1.0), 0 means fixed width
	MinWidth   int            // Minimum width in characters
	MaxWidth   int            // Maximum width (0 = no limit)
	Align      Alignment      // Text alignment
	Style      lipgloss.Style // Style to apply to cells in this column
}

// ResultRow represents a single row in the results table.
type ResultRow struct {
	Num      int      // Row number (1-indexed)
	Cells    []string // Cell values for each column
	Location string   // Location for hyperlinks (file:line format)
}

// ResultsTable provides a unified table renderer for all retrieval types.
type ResultsTable struct {
	display *DisplayContext
	columns []ColumnDef
	rows    []ResultRow
}

// Standard column definitions shared across retrieval types.
var (
	// ColNum is the row number column (fixed width, right-aligned, muted).
	ColNum = ColumnDef{
		Name:     "num",
		MinWidth: 4,
		MaxWidth: 6,
		Align:    AlignRight,
		Style:    Muted,
	}

	// ColContent is the main content column (flexible width).
	ColContent = ColumnDef{
		Name:       "content",
		WidthRatio: 0.55,
		MinWidth:   30,
		MaxWidth:   100,
		Align:      AlignLeft,
	}

	// ColMeta is the metadata column (trait info, title, etc).
	ColMeta = ColumnDef{
		Name:       "meta",
		WidthRatio: 0.25,
		MinWidth:   15,
		MaxWidth:   35,
		Align:      AlignLeft,
		Style:      Muted,
	}

	// ColFile is the file location column.
	ColFile = ColumnDef{
		Name:       "file",
		WidthRatio: 0.20,
		MinWidth:   10,
		MaxWidth:   30,
		Align:      AlignLeft,
		Style:      Muted,
	}

	// ColBacklinksMeta is a wider metadata/content column used for backlinks/outlinks.
	ColBacklinksMeta = ColumnDef{
		Name:       "meta",
		WidthRatio: 0.70,
		MinWidth:   30,
		MaxWidth:   120,
		Align:      AlignLeft,
		Style:      Muted,
	}

	// ColBacklinksFile is a wider file column used for backlinks/outlinks.
	ColBacklinksFile = ColumnDef{
		Name:       "file",
		WidthRatio: 0.30,
		MinWidth:   18,
		MaxWidth:   60,
		Align:      AlignLeft,
		Style:      Muted,
	}
)

// Standard layouts for each retrieval type.
var (
	// SearchLayout is used for search results: [num, content, meta, file]
	SearchLayout = []ColumnDef{ColNum, ColContent, ColMeta, ColFile}

	// TraitLayout is used for trait query results: [num, content, meta, file]
	TraitLayout = []ColumnDef{ColNum, ColContent, ColMeta, ColFile}

	// BacklinksLayout is used for backlinks: [num, meta, file]
	BacklinksLayout = []ColumnDef{ColNum, ColBacklinksMeta, ColBacklinksFile}
)

// NewResultsTable creates a new ResultsTable with the given display context and column layout.
func NewResultsTable(display *DisplayContext, columns []ColumnDef) *ResultsTable {
	return &ResultsTable{
		display: display,
		columns: columns,
		rows:    make([]ResultRow, 0),
	}
}

// AddRow adds a row to the table.
func (t *ResultsTable) AddRow(row ResultRow) {
	t.rows = append(t.rows, row)
}

// ContentWidth returns the calculated width for a specific column by name.
// This allows callers to prepare content (e.g., snippet extraction) based on actual available width.
func (t *ResultsTable) ContentWidth(columnName string) int {
	widths := t.calculateWidths()
	for i, col := range t.columns {
		if col.Name == columnName {
			return widths[i]
		}
	}
	return 60 // fallback
}

// GetColumnWidth returns the calculated width for a column by index.
func (t *ResultsTable) GetColumnWidth(index int) int {
	widths := t.calculateWidths()
	if index >= 0 && index < len(widths) {
		return widths[index]
	}
	return 60 // fallback
}

// calculateWidths computes column widths based on terminal size and column definitions.
func (t *ResultsTable) calculateWidths() []int {
	widths := make([]int, len(t.columns))

	// First pass: calculate fixed widths and total ratio
	var totalRatio float64
	var fixedWidth int
	const columnPadding = 2 // padding between columns

	for i, col := range t.columns {
		if col.WidthRatio == 0 {
			// Fixed-width column: use MinWidth or calculate from content
			widths[i] = col.MinWidth
			if col.MaxWidth > 0 && widths[i] > col.MaxWidth {
				widths[i] = col.MaxWidth
			}
			fixedWidth += widths[i]
		} else {
			totalRatio += col.WidthRatio
		}
	}

	// Calculate available space for flexible columns
	totalPadding := (len(t.columns) - 1) * columnPadding
	leftMargin := 2 // indent for aesthetic
	available := t.display.TermWidth - fixedWidth - totalPadding - leftMargin

	if available < 0 {
		available = 0
	}

	// Second pass: distribute available space by ratio
	for i, col := range t.columns {
		if col.WidthRatio > 0 {
			// Calculate proportional width
			ratio := col.WidthRatio / totalRatio
			width := int(float64(available) * ratio)

			// Apply min/max constraints
			if width < col.MinWidth {
				width = col.MinWidth
			}
			if col.MaxWidth > 0 && width > col.MaxWidth {
				width = col.MaxWidth
			}

			widths[i] = width
		}
	}

	return widths
}

// Render generates the table output as a string.
func (t *ResultsTable) Render() string {
	if len(t.rows) == 0 {
		return ""
	}

	widths := t.calculateWidths()

	// Build table data
	tableRows := make([][]string, len(t.rows))
	for i, row := range t.rows {
		tableRow := make([]string, len(t.columns))
		for j := range t.columns {
			if j < len(row.Cells) {
				tableRow[j] = row.Cells[j]
			}
		}
		tableRows[i] = tableRow
	}

	// Create lipgloss table with minimal border style
	tbl := table.New().
		Border(lipgloss.Border{
			Top:    "─",
			Bottom: "─",
			Left:   "",
			Right:  "",
			Middle: "─",
		}).
		BorderTop(false).
		BorderBottom(false).
		BorderLeft(false).
		BorderRight(false).
		BorderRow(true).
		BorderColumn(false).
		BorderStyle(Muted).
		StyleFunc(func(row, col int) lipgloss.Style {
			if col >= len(t.columns) {
				return lipgloss.NewStyle()
			}

			colDef := t.columns[col]
			style := colDef.Style
			if style.Value() == "" {
				style = lipgloss.NewStyle()
			}

			// Set width
			style = style.Width(widths[col])

			// Set alignment
			switch colDef.Align {
			case AlignRight:
				style = style.Align(lipgloss.Right)
			case AlignCenter:
				style = style.Align(lipgloss.Center)
			default:
				style = style.Align(lipgloss.Left)
			}

			// Add right padding except for last column
			if col < len(t.columns)-1 {
				style = style.PaddingRight(2)
			}

			return style
		}).
		Rows(tableRows...)

	return tbl.Render()
}

// TruncateWithEllipsis truncates a string to maxLen, adding ellipsis if needed.
// It tries to break at word boundaries.
func TruncateWithEllipsis(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return s[:maxLen]
	}

	// Try to truncate at a word boundary
	truncated := s[:maxLen-3]
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}

// WrapTextTwoLines wraps text into at most two lines, with the second line truncated.
func WrapTextTwoLines(text string, maxLen int) (line1, line2 string) {
	if len(text) <= maxLen {
		return text, ""
	}

	// Find a good break point near maxLen
	breakPoint := maxLen
	for i := maxLen; i > maxLen/2; i-- {
		if i < len(text) && text[i] == ' ' {
			breakPoint = i
			break
		}
	}

	line1 = strings.TrimSpace(text[:breakPoint])
	line2 = strings.TrimSpace(text[breakPoint:])

	// Truncate line2 if still too long
	if len(line2) > maxLen {
		line2 = TruncateWithEllipsis(line2, maxLen)
	}

	return line1, line2
}

// FormatRowNum formats a row number with consistent width.
func FormatRowNum(num, maxNum int) string {
	width := len(fmt.Sprintf("%d", maxNum))
	if width < 2 {
		width = 2
	}
	return fmt.Sprintf("%*d", width, num)
}
