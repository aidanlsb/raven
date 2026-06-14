package ui

import (
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/charmbracelet/x/ansi"
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
	HasStyle   bool           // Whether Style should be applied to cells in this column
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
	headers []string
	rows    []ResultRow
}

// Standard column definitions shared across retrieval types.
func colNum() ColumnDef {
	return ColumnDef{
		Name:     "num",
		MinWidth: 4,
		MaxWidth: 6,
		Align:    AlignRight,
		HasStyle: true,
		Style:    Muted,
	}
}

func colContent() ColumnDef {
	return ColumnDef{
		Name:       "content",
		WidthRatio: 0.55,
		MinWidth:   30,
		MaxWidth:   100,
		Align:      AlignLeft,
	}
}

func colMeta() ColumnDef {
	return ColumnDef{
		Name:       "meta",
		WidthRatio: 0.25,
		MinWidth:   15,
		MaxWidth:   35,
		Align:      AlignLeft,
		HasStyle:   true,
		Style:      Muted,
	}
}

func colFile() ColumnDef {
	return ColumnDef{
		Name:       "file",
		WidthRatio: 0.20,
		MinWidth:   10,
		MaxWidth:   30,
		Align:      AlignLeft,
		HasStyle:   true,
		Style:      Muted,
	}
}

func colBacklinksContent() ColumnDef {
	return ColumnDef{
		Name:       "content",
		WidthRatio: 0.70,
		MinWidth:   30,
		MaxWidth:   120,
		Align:      AlignLeft,
	}
}

func colBacklinksFile() ColumnDef {
	return ColumnDef{
		Name:       "file",
		WidthRatio: 0.30,
		MinWidth:   18,
		MaxWidth:   60,
		Align:      AlignLeft,
		HasStyle:   true,
		Style:      Muted,
	}
}

func colObjectName() ColumnDef {
	return ColumnDef{
		Name:       "name",
		WidthRatio: 1,
		MinWidth:   20,
		Align:      AlignLeft,
	}
}

func colObjectField(name string) ColumnDef {
	minWidth := len(name)
	if minWidth < 4 {
		minWidth = 4
	}
	return ColumnDef{
		Name:       "field:" + name,
		WidthRatio: 1,
		MinWidth:   minWidth,
		Align:      AlignLeft,
	}
}

func colObjectLocation() ColumnDef {
	return ColumnDef{
		Name:       "location",
		WidthRatio: 1,
		MinWidth:   18,
		Align:      AlignLeft,
		HasStyle:   true,
		Style:      Muted,
	}
}

// SearchLayout returns the standard search results layout: [num, content, meta, file].
func SearchLayout() []ColumnDef {
	return []ColumnDef{colNum(), colContent(), colMeta(), colFile()}
}

// TraitLayout returns the standard trait query results layout: [num, content, meta, file].
func TraitLayout() []ColumnDef {
	return []ColumnDef{colNum(), colContent(), colMeta(), colFile()}
}

// BacklinksLayout returns the standard backlinks layout: [num, content, file].
func BacklinksLayout() []ColumnDef {
	return []ColumnDef{colNum(), colBacklinksContent(), colBacklinksFile()}
}

// AssetLayout returns the standard asset query layout: [num, path, media type, size].
func AssetLayout() []ColumnDef {
	return []ColumnDef{colNum(), colContent(), colMeta(), colFile()}
}

// ObjectLayout returns the standard object query layout:
// [num, name, dynamic fields..., location].
func ObjectLayout(fieldNames []string) []ColumnDef {
	columns := make([]ColumnDef, 0, len(fieldNames)+3)
	columns = append(columns, colNum(), colObjectName())
	for _, fieldName := range fieldNames {
		columns = append(columns, colObjectField(fieldName))
	}
	columns = append(columns, colObjectLocation())
	return columns
}

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

// SetHeaders configures an optional header row for human-readable tables.
func (t *ResultsTable) SetHeaders(headers []string) {
	t.headers = headers
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

// CalculateColumnWidths computes column widths from shared table definitions.
func CalculateColumnWidths(columns []ColumnDef, termWidth int) []int {
	widths := make([]int, len(columns))

	// First pass: calculate fixed widths and total ratio
	var totalRatio float64
	var fixedWidth int
	const columnPadding = 2 // padding between columns

	for i, col := range columns {
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
	totalPadding := (len(columns) - 1) * columnPadding
	leftMargin := 2 // indent for aesthetic
	available := termWidth - fixedWidth - totalPadding - leftMargin

	if available < 0 {
		available = 0
	}

	// Second pass: distribute available space by ratio
	for i, col := range columns {
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

	fitWidthsToAvailable(widths, columns, termWidth-totalPadding-leftMargin)
	return widths
}

// CalculateColumnWidthsForRows computes column widths from actual rendered data.
func CalculateColumnWidthsForRows(columns []ColumnDef, headers []string, rows [][]string, termWidth int) []int {
	if len(columns) == 0 {
		return nil
	}
	if len(headers) == 0 && len(rows) == 0 {
		return CalculateColumnWidths(columns, termWidth)
	}

	widths := make([]int, len(columns))
	for i, col := range columns {
		widths[i] = desiredColumnWidth(col, columnValues(i, headers, rows))
	}

	const columnPadding = 2
	totalPadding := (len(columns) - 1) * columnPadding
	leftMargin := 2
	fitWidthsToAvailable(widths, columns, termWidth-totalPadding-leftMargin)
	return widths
}

func columnValues(index int, headers []string, rows [][]string) []string {
	values := make([]string, 0, len(rows)+1)
	if index < len(headers) {
		values = append(values, headers[index])
	}
	for _, row := range rows {
		if index < len(row) {
			values = append(values, row[index])
		}
	}
	return values
}

func desiredColumnWidth(col ColumnDef, values []string) int {
	minWidth := col.MinWidth
	if minWidth < 1 {
		minWidth = 1
	}
	if col.MaxWidth > 0 && minWidth > col.MaxWidth {
		minWidth = col.MaxWidth
	}

	widths := make([]int, 0, len(values))
	for _, value := range values {
		for _, line := range strings.Split(value, "\n") {
			if width := VisibleLen(line); width > 0 {
				widths = append(widths, width)
			}
		}
	}
	if len(widths) == 0 {
		return minWidth
	}
	sort.Ints(widths)
	desired := widths[percentileIndex(len(widths), 85)]
	if desired < minWidth {
		desired = minWidth
	}
	if col.MaxWidth > 0 && desired > col.MaxWidth {
		desired = col.MaxWidth
	}
	return desired
}

func percentileIndex(length int, percentile int) int {
	if length <= 1 {
		return 0
	}
	index := (length*percentile + 99) / 100
	if index < 1 {
		index = 1
	}
	if index > length {
		index = length
	}
	return index - 1
}

func fitWidthsToAvailable(widths []int, columns []ColumnDef, available int) {
	if available < 0 {
		available = 0
	}
	used := 0
	for _, width := range widths {
		used += width
	}
	for used > available {
		shrinkIndex := -1
		for i, width := range widths {
			minWidth := columns[i].MinWidth
			if width <= minWidth {
				continue
			}
			if shrinkIndex == -1 ||
				columnShrinkPriority(columns[i]) < columnShrinkPriority(columns[shrinkIndex]) ||
				(columnShrinkPriority(columns[i]) == columnShrinkPriority(columns[shrinkIndex]) && width > widths[shrinkIndex]) {
				shrinkIndex = i
			}
		}
		if shrinkIndex == -1 {
			return
		}
		widths[shrinkIndex]--
		used--
	}
}

func columnShrinkPriority(col ColumnDef) int {
	switch {
	case col.Name == "num":
		return 100
	case col.Name == "name" || col.Name == "content":
		return 80
	case strings.HasPrefix(col.Name, "field:") || col.Name == "meta":
		return 50
	case col.Name == "file" || col.Name == "location":
		return 10
	default:
		return 40
	}
}

// calculateWidths computes column widths based on terminal size and column definitions.
func (t *ResultsTable) calculateWidths() []int {
	return CalculateColumnWidths(t.columns, t.display.TermWidth)
}

// Render generates the table output as a string.
func (t *ResultsTable) Render() string {
	if len(t.rows) == 0 {
		return ""
	}

	rowCells := make([][]string, 0, len(t.rows))
	for _, row := range t.rows {
		tableRow := make([]string, len(t.columns))
		for j := range t.columns {
			if j < len(row.Cells) {
				tableRow[j] = row.Cells[j]
			}
		}
		rowCells = append(rowCells, tableRow)
	}
	widths := CalculateColumnWidthsForRows(t.columns, t.headers, rowCells, t.display.TermWidth)

	tableRows := make([][]string, 0, len(t.rows)+1)
	if len(t.headers) > 0 {
		headerRow := make([]string, len(t.columns))
		for i := range t.columns {
			if i < len(t.headers) {
				headerRow[i] = truncateCell(t.headers[i], widths[i])
			}
		}
		tableRows = append(tableRows, headerRow)
	}
	for _, row := range rowCells {
		tableRow := make([]string, len(t.columns))
		for j := range t.columns {
			tableRow[j] = truncateCell(row[j], widths[j])
		}
		tableRows = append(tableRows, tableRow)
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
			style := lipgloss.NewStyle()
			if len(t.headers) > 0 && row == 0 {
				style = Muted.Bold(true)
			} else if colDef.HasStyle {
				style = colDef.Style
			}

			// Set width. Column widths represent usable content width; inter-column
			// padding is accounted for separately in CalculateColumnWidths.
			styleWidth := widths[col]
			if col < len(t.columns)-1 {
				styleWidth += 2
			}
			style = style.Width(styleWidth)

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

func truncateCell(s string, width int) string {
	if width < 1 {
		return ""
	}
	if VisibleLen(s) <= width {
		return s
	}
	if width <= 3 {
		return ansi.Truncate(s, width, "")
	}
	return ansi.Truncate(s, width, "...")
}

// TruncateWithEllipsis truncates a string to maxLen, adding ellipsis if needed.
// It tries to break at word boundaries.
func TruncateWithEllipsis(s string, maxLen int) string {
	runes := []rune(s)
	if len(runes) <= maxLen {
		return s
	}
	if maxLen <= 3 {
		return string(runes[:maxLen])
	}

	// Try to truncate at a word boundary
	truncated := string(runes[:maxLen-3])
	lastSpace := strings.LastIndex(truncated, " ")
	if lastSpace > maxLen/2 {
		truncated = truncated[:lastSpace]
	}
	return truncated + "..."
}

// WrapTextTwoLines wraps text into at most two lines, with the second line truncated.
func WrapTextTwoLines(text string, maxLen int) (line1, line2 string) {
	runes := []rune(text)
	if len(runes) <= maxLen {
		return text, ""
	}

	// Find a good break point near maxLen
	breakPoint := maxLen
	for i := maxLen; i > maxLen/2; i-- {
		if i < len(runes) && runes[i] == ' ' {
			breakPoint = i
			break
		}
	}

	line1 = strings.TrimSpace(string(runes[:breakPoint]))
	line2 = strings.TrimSpace(string(runes[breakPoint:]))

	// Truncate line2 if still too long
	if len([]rune(line2)) > maxLen {
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
