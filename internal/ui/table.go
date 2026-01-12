package ui

import (
	"strings"
)

// Table provides minimal table/list rendering
// Uses simple spacing alignment without borders

// Table represents a simple table structure
type Table struct {
	rows       [][]string
	colWidths  []int
	colPadding int
}

// NewTable creates a new table with the specified number of columns
func NewTable(cols int) *Table {
	return &Table{
		colWidths:  make([]int, cols),
		colPadding: 2,
	}
}

// AddRow adds a row to the table
func (t *Table) AddRow(cells ...string) {
	// Ensure we have the right number of cells
	row := make([]string, len(t.colWidths))
	for i := 0; i < len(t.colWidths) && i < len(cells); i++ {
		row[i] = cells[i]
		// Track max width for each column
		if len(cells[i]) > t.colWidths[i] {
			t.colWidths[i] = len(cells[i])
		}
	}
	t.rows = append(t.rows, row)
}

// SetPadding sets the padding between columns
func (t *Table) SetPadding(padding int) {
	t.colPadding = padding
}

// String renders the table as a string
func (t *Table) String() string {
	if len(t.rows) == 0 {
		return ""
	}

	var sb strings.Builder
	padding := strings.Repeat(" ", t.colPadding)

	for _, row := range t.rows {
		for i, cell := range row {
			if i > 0 {
				sb.WriteString(padding)
			}
			// Left-align all columns, pad to column width (except last)
			if i < len(row)-1 {
				sb.WriteString(cell)
				sb.WriteString(strings.Repeat(" ", t.colWidths[i]-len(cell)))
			} else {
				sb.WriteString(cell)
			}
		}
		sb.WriteString("\n")
	}

	return sb.String()
}

// List provides a simple indented list renderer
type List struct {
	items  []string
	indent string
	bullet string
}

// NewList creates a new list with default settings
func NewList() *List {
	return &List{
		indent: "  ",
		bullet: "•",
	}
}

// SetIndent sets the indentation string
func (l *List) SetIndent(indent string) {
	l.indent = indent
}

// SetBullet sets the bullet character
func (l *List) SetBullet(bullet string) {
	l.bullet = bullet
}

// Add adds an item to the list
func (l *List) Add(item string) {
	l.items = append(l.items, item)
}

// String renders the list as a string
func (l *List) String() string {
	var sb strings.Builder
	for _, item := range l.items {
		sb.WriteString(l.indent)
		sb.WriteString(l.bullet)
		sb.WriteString(" ")
		sb.WriteString(item)
		sb.WriteString("\n")
	}
	return sb.String()
}
