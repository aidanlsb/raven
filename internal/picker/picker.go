package picker

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"
)

// Item is a selectable entry in a Raven-owned interactive picker.
type Item struct {
	ID         string
	Label      string
	Detail     string
	Location   string
	Columns    []string
	SearchText string
	FilePath   string
	Line       int
}

// Options controls picker copy and layout.
type Options struct {
	Title   string
	Prompt  string
	Headers []string
}

// Selection is the item selected by the user.
type Selection struct {
	Item Item
}

type inputMode int

const (
	normalMode inputMode = iota
	insertMode
)

// Run starts an interactive picker and returns the selected item.
func Run(items []Item, opts Options) (Selection, bool, error) {
	if len(items) == 0 {
		return Selection{}, false, nil
	}

	initial := newModel(items, opts)
	finalModel, err := tea.NewProgram(initial, tea.WithAltScreen()).Run()
	if err != nil {
		return Selection{}, false, err
	}

	m, ok := finalModel.(model)
	if !ok || !m.selected {
		return Selection{}, false, nil
	}
	return Selection{Item: m.items[m.filtered[m.cursor]]}, true, nil
}

type model struct {
	items    []Item
	filtered []int
	opts     Options

	query    string
	cursor   int
	offset   int
	width    int
	height   int
	selected bool
	mode     inputMode
	pendingG bool
}

func newModel(items []Item, opts Options) model {
	m := model{
		items:  items,
		opts:   opts,
		width:  100,
		height: 30,
	}
	m.applyFilter()
	return m
}

func (m model) Init() tea.Cmd {
	return nil
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.clamp()
	case tea.KeyMsg:
		var cmd tea.Cmd
		m, cmd = m.updateKey(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) updateKey(msg tea.KeyMsg) (model, tea.Cmd) {
	if msg.Type == tea.KeyCtrlC {
		return m, tea.Quit
	}
	if msg.Type == tea.KeyEnter {
		if len(m.filtered) > 0 {
			m.selected = true
		}
		return m, tea.Quit
	}
	if m.mode == insertMode {
		return m.updateInsertKey(msg)
	}
	return m.updateNormalKey(msg)
}

func (m model) updateNormalKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.pendingG = false
		return m, nil
	case tea.KeyUp:
		m.pendingG = false
		m.moveCursor(-1)
	case tea.KeyDown:
		m.pendingG = false
		m.moveCursor(1)
	case tea.KeyPgUp:
		m.pendingG = false
		m.moveCursor(-m.listHeight())
	case tea.KeyPgDown:
		m.pendingG = false
		m.moveCursor(m.listHeight())
	case tea.KeyRunes:
		switch msg.String() {
		case "i", "/":
			m.pendingG = false
			m.mode = insertMode
		case "g":
			if m.pendingG {
				m.pendingG = false
				m.moveToTop()
			} else {
				m.pendingG = true
			}
		case "G":
			m.pendingG = false
			m.moveToBottom()
		case "j":
			m.pendingG = false
			m.moveCursor(1)
		case "k":
			m.pendingG = false
			m.moveCursor(-1)
		case "q":
			return m, tea.Quit
		default:
			m.pendingG = false
		}
	}
	return m, nil
}

func (m model) updateInsertKey(msg tea.KeyMsg) (model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEsc:
		m.mode = normalMode
	case tea.KeyBackspace, tea.KeyDelete:
		if m.query != "" {
			m.query = dropLastRune(m.query)
			m.applyFilter()
		}
	case tea.KeyCtrlU:
		if m.query != "" {
			m.query = ""
			m.applyFilter()
		}
	case tea.KeyCtrlW:
		next := dropLastWord(m.query)
		if next != m.query {
			m.query = next
			m.applyFilter()
		}
	case tea.KeyRunes:
		m.query += msg.String()
		m.applyFilter()
	}
	return m, nil
}

func (m model) View() string {
	title := strings.TrimSpace(m.opts.Title)
	if title == "" {
		title = "Select"
	}
	prompt := strings.TrimSpace(m.opts.Prompt)
	if prompt == "" {
		prompt = "filter"
	}

	header := titleStyle.Render(title)
	filter := mutedStyle.Render(fmt.Sprintf("%s [%s]: ", prompt, m.modeLabel())) + m.query
	help := mutedStyle.Render(m.helpText())

	bodyHeight := m.height - 4
	if bodyHeight < 8 {
		bodyHeight = 8
	}
	body := lipgloss.NewStyle().Width(m.width).Height(bodyHeight).Render(m.renderList(m.width))

	return strings.Join([]string{header, filter, body, help}, "\n")
}

func (m model) modeLabel() string {
	if m.mode == insertMode {
		return "INSERT"
	}
	return "NORMAL"
}

func (m model) helpText() string {
	if m.mode == insertMode {
		return "insert: type filter  esc: normal  ctrl-w: delete word  ctrl-u: clear"
	}
	return "normal: j/k move  gg/G top/bottom  / or i: filter  enter: open  q: cancel"
}

func (m model) renderList(width int) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("No matches")
	}
	if len(m.opts.Headers) > 0 {
		return m.renderTableList(width)
	}

	height := m.listHeight()
	end := m.offset + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	lines := make([]string, 0, end-m.offset)
	for visibleIndex, filteredIndex := range m.filtered[m.offset:end] {
		item := m.items[filteredIndex]
		line := item.Label
		if item.Detail != "" {
			line += "  " + mutedStyle.Render(item.Detail)
		}
		if item.Location != "" {
			line += "  " + mutedStyle.Render(item.Location)
		}
		if m.offset+visibleIndex == m.cursor {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
		if visibleIndex < end-m.offset-1 {
			lines = append(lines, rowDivider(width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) renderTableList(width int) string {
	height := m.listHeight()
	end := m.offset + height
	if end > len(m.filtered) {
		end = len(m.filtered)
	}

	headers := m.opts.Headers
	widths := tableWidths(headers, width)
	lines := []string{mutedStyle.Render(formatTableRow(headers, widths)), rowDivider(width)}

	for visibleIndex, filteredIndex := range m.filtered[m.offset:end] {
		item := m.items[filteredIndex]
		row := make([]string, 0, len(headers))
		num := fmt.Sprintf("%d", m.offset+visibleIndex+1)
		if m.offset+visibleIndex == m.cursor {
			num = ">" + num
		}
		row = append(row, num)
		row = append(row, item.Columns...)
		for len(row) < len(headers) {
			row = append(row, "")
		}
		if len(row) > len(headers) {
			row = row[:len(headers)]
		}

		rendered := formatTableRow(row, widths)
		if m.offset+visibleIndex == m.cursor {
			rendered = selectedStyle.Render(rendered)
		}
		lines = append(lines, rendered)
		if visibleIndex < end-m.offset-1 {
			lines = append(lines, rowDivider(width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) listHeight() int {
	bodyHeight := m.height - 4
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	if len(m.opts.Headers) > 0 {
		// Header + header divider take two lines; rows are separated by
		// dividers, so N rows render as 2N+1 lines.
		height := (bodyHeight - 1) / 2
		if height < 1 {
			return 1
		}
		return height
	}

	// Rows are separated by dividers, so N rows render as 2N-1 lines.
	height := (bodyHeight + 1) / 2
	if height < 1 {
		return 1
	}
	return height
}

func (m *model) applyFilter() {
	m.filtered = rankItems(m.items, m.query)
	m.cursor = 0
	m.offset = 0
	m.clamp()
}

func (m *model) moveCursor(delta int) {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor += delta
	m.clamp()
}

func (m *model) moveToTop() {
	m.cursor = 0
	m.clamp()
}

func (m *model) moveToBottom() {
	if len(m.filtered) == 0 {
		return
	}
	m.cursor = len(m.filtered) - 1
	m.clamp()
}

func (m *model) clamp() {
	if len(m.filtered) == 0 {
		m.cursor = 0
		m.offset = 0
		return
	}
	if m.cursor < 0 {
		m.cursor = 0
	}
	if m.cursor >= len(m.filtered) {
		m.cursor = len(m.filtered) - 1
	}

	height := m.listHeight()
	if m.cursor < m.offset {
		m.offset = m.cursor
	}
	if m.cursor >= m.offset+height {
		m.offset = m.cursor - height + 1
	}
	if m.offset < 0 {
		m.offset = 0
	}
}

func rankItems(items []Item, query string) []int {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		indexes := make([]int, len(items))
		for i := range items {
			indexes[i] = i
		}
		return indexes
	}

	targets := make([]string, len(items))
	for i, item := range items {
		targets[i] = item.searchText()
	}
	matches := fuzzy.Find(query, targets)
	sort.Stable(matches)

	indexes := make([]int, 0, len(matches))
	for _, match := range matches {
		indexes = append(indexes, match.Index)
	}
	return indexes
}

func (item Item) searchText() string {
	if strings.TrimSpace(item.SearchText) != "" {
		return item.SearchText
	}
	return strings.Join([]string{
		item.Label,
		item.Detail,
		item.Location,
		item.ID,
		item.FilePath,
		strings.Join(item.Columns, " "),
	}, " ")
}

func tableWidths(headers []string, totalWidth int) []int {
	if len(headers) == 0 {
		return nil
	}
	if totalWidth < 20 {
		totalWidth = 20
	}
	const padding = 2
	widths := make([]int, len(headers))
	widths[0] = 4

	remaining := totalWidth - widths[0] - padding*(len(headers)-1)
	if remaining < len(headers)-1 {
		remaining = len(headers) - 1
	}
	if len(headers) == 1 {
		return widths
	}

	weights := make([]int, len(headers)-1)
	totalWeight := 0
	for i := 1; i < len(headers); i++ {
		weight := 1
		if i == 1 || i == len(headers)-1 {
			weight = 2
		}
		weights[i-1] = weight
		totalWeight += weight
	}

	used := 0
	for i := 1; i < len(headers); i++ {
		width := remaining * weights[i-1] / totalWeight
		if width < 6 {
			width = 6
		}
		widths[i] = width
		used += width
	}
	for used > remaining {
		largest := 1
		for i := 2; i < len(widths); i++ {
			if widths[i] > widths[largest] {
				largest = i
			}
		}
		if widths[largest] <= 4 {
			break
		}
		widths[largest]--
		used--
	}
	for used < remaining {
		widths[len(widths)-1]++
		used++
	}
	return widths
}

func formatTableRow(cells []string, widths []int) string {
	parts := make([]string, 0, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = truncate(cell, width)
		if i == 0 {
			parts = append(parts, fmt.Sprintf("%*s", width, cell))
		} else {
			parts = append(parts, fmt.Sprintf("%-*s", width, cell))
		}
	}
	return strings.Join(parts, "  ")
}

func truncate(s string, width int) string {
	runes := []rune(s)
	if len(runes) <= width {
		return s
	}
	if width <= 3 {
		return string(runes[:width])
	}
	return string(runes[:width-3]) + "..."
}

func rowDivider(width int) string {
	if width < 1 {
		width = 1
	}
	return mutedStyle.Render(strings.Repeat("─", width))
}

func dropLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

func dropLastWord(s string) string {
	runes := []rune(s)
	i := len(runes)
	for i > 0 && unicode.IsSpace(runes[i-1]) {
		i--
	}
	for i > 0 && !unicode.IsSpace(runes[i-1]) {
		i--
	}
	return string(runes[:i])
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
