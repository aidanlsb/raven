package picker

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"unicode"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sahilm/fuzzy"

	"github.com/aidanlsb/raven/internal/ui"
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
	Title        string
	Prompt       string
	Headers      []string
	Columns      []ui.ColumnDef
	MultiSelect  bool
	AllowForward bool
	AllowBack    bool
	Shortcuts    []ShortcutTip
	Input        io.Reader
	Output       io.Writer
}

// ShortcutTip describes an optional picker gutter shortcut.
type ShortcutTip struct {
	Key         string
	Description string
}

// Action describes how the picker was completed.
type Action string

const (
	ActionSelect  Action = ""
	ActionForward Action = "forward"
	ActionBack    Action = "back"
)

// Selection is the item selected by the user.
type Selection struct {
	Item   Item
	Action Action
}

type inputMode int

const (
	normalMode inputMode = iota
	insertMode
)

// Run starts an interactive picker and returns the selected item.
func Run(items []Item, opts Options) (Selection, bool, error) {
	selections, ok, err := run(items, opts)
	if err != nil || !ok || len(selections) == 0 {
		return Selection{}, ok, err
	}
	return selections[0], true, nil
}

// RunMulti starts an interactive picker and returns one or more selected items.
func RunMulti(items []Item, opts Options) ([]Selection, bool, error) {
	opts.MultiSelect = true
	return run(items, opts)
}

func run(items []Item, opts Options) ([]Selection, bool, error) {
	if len(items) == 0 {
		return nil, false, nil
	}

	initial := newModel(items, opts)
	programOptions := []tea.ProgramOption{tea.WithAltScreen()}
	if opts.Input != nil {
		programOptions = append(programOptions, tea.WithInput(opts.Input))
	}
	if opts.Output != nil {
		programOptions = append(programOptions, tea.WithOutput(opts.Output))
	}
	finalModel, err := tea.NewProgram(initial, programOptions...).Run()
	if err != nil {
		return nil, false, err
	}

	m, ok := finalModel.(model)
	if !ok || !m.selected {
		return nil, false, nil
	}
	return m.selections(), true, nil
}

type model struct {
	items    []Item
	filtered []int
	opts     Options

	query        string
	cursor       int
	offset       int
	width        int
	height       int
	selected     bool
	action       Action
	mode         inputMode
	pendingG     bool
	selectedKeys map[string]bool
}

func newModel(items []Item, opts Options) model {
	m := model{
		items:        items,
		opts:         opts,
		width:        100,
		height:       30,
		selectedKeys: make(map[string]bool),
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
			m.action = ActionSelect
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
	case tea.KeySpace:
		m.pendingG = false
		m.toggleCurrent()
	case tea.KeyRunes:
		switch msg.String() {
		case " ":
			m.pendingG = false
			m.toggleCurrent()
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
		case "h", "H":
			m.pendingG = false
			if m.opts.AllowBack {
				m.selected = true
				m.action = ActionBack
				return m, tea.Quit
			}
		case "j":
			m.pendingG = false
			m.moveCursor(1)
		case "k":
			m.pendingG = false
			m.moveCursor(-1)
		case "l", "L":
			m.pendingG = false
			if m.opts.AllowForward && len(m.filtered) > 0 {
				m.selected = true
				m.action = ActionForward
				return m, tea.Quit
			}
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
	body := m.renderBody(bodyHeight)

	return strings.Join([]string{header, filter, body, help}, "\n")
}

func (m model) renderBody(bodyHeight int) string {
	gutter := m.renderShortcutGutter()
	if gutter == "" {
		return lipgloss.NewStyle().Width(m.width).Height(bodyHeight).Render(m.renderList(m.width))
	}

	gutterWidth := m.shortcutGutterWidth()
	separatorWidth := 3
	minListWidth := 40
	if m.width < minListWidth+gutterWidth+separatorWidth {
		return lipgloss.NewStyle().Width(m.width).Height(bodyHeight).Render(m.renderList(m.width))
	}

	listWidth := m.width - gutterWidth - separatorWidth
	list := lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(m.renderList(listWidth))
	separator := mutedStyle.Render(" │ ")
	sidebar := lipgloss.NewStyle().Width(gutterWidth).Height(bodyHeight).Render(gutter)
	return lipgloss.JoinHorizontal(lipgloss.Top, list, separator, sidebar)
}

func (m model) renderShortcutGutter() string {
	if len(m.opts.Shortcuts) == 0 {
		return ""
	}

	lines := []string{mutedStyle.Bold(true).Render("shortcuts")}
	for _, shortcut := range m.opts.Shortcuts {
		key := strings.TrimSpace(shortcut.Key)
		description := strings.TrimSpace(shortcut.Description)
		if key == "" || description == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%-9s %s", key, description))
	}
	if len(lines) == 1 {
		return ""
	}
	return mutedStyle.Render(strings.Join(lines, "\n"))
}

func (m model) shortcutGutterWidth() int {
	width := len("shortcuts")
	for _, shortcut := range m.opts.Shortcuts {
		key := strings.TrimSpace(shortcut.Key)
		description := strings.TrimSpace(shortcut.Description)
		if key == "" || description == "" {
			continue
		}
		lineWidth := len([]rune(key)) + 1 + len([]rune(description))
		if lineWidth > width {
			width = lineWidth
		}
	}
	if width < 16 {
		return 16
	}
	if width > 28 {
		return 28
	}
	return width
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
	nav := ""
	if m.opts.AllowBack && m.opts.AllowForward {
		nav = "  h/l: back/forward"
	} else if m.opts.AllowBack {
		nav = "  h: back"
	} else if m.opts.AllowForward {
		nav = "  l: forward"
	}
	if m.opts.MultiSelect {
		return fmt.Sprintf("normal: j/k move%s  space: toggle  enter: select  selected: %d  q: cancel", nav, m.selectedCount())
	}
	return fmt.Sprintf("normal: j/k move  gg/G top/bottom%s  / or i: filter  enter: open  q: cancel", nav)
}

func (m model) renderList(width int) string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("No matches")
	}
	if len(m.tableColumns()) > 0 {
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
			prefix := ui.SymbolAttention + " "
			if m.isSelected(filteredIndex) {
				prefix += ui.SymbolCheck + " "
			}
			line = selectedStyle.Render(prefix + line)
		} else if m.isSelected(filteredIndex) {
			line = ui.SymbolCheck + " " + line
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

	columns := m.tableColumns()
	headers := m.tableHeaders(columns)
	widths := ui.CalculateColumnWidthsForRows(columns, headers, m.tableRowsForWidths(columns), width)
	lines := []string{formatTableRow(headers, widths, columns, true, false), rowDivider(width)}

	for visibleIndex, filteredIndex := range m.filtered[m.offset:end] {
		item := m.items[filteredIndex]
		row := make([]string, 0, len(columns))
		num := ui.FormatRowNum(m.offset+visibleIndex+1, len(m.filtered))
		if m.offset+visibleIndex == m.cursor {
			num = ui.SymbolAttention + " " + strings.TrimSpace(num)
		} else if m.isSelected(filteredIndex) {
			num = ui.SymbolCheck + " " + strings.TrimSpace(num)
		}
		row = append(row, num)
		row = append(row, item.Columns...)
		for len(row) < len(columns) {
			row = append(row, "")
		}
		if len(row) > len(columns) {
			row = row[:len(columns)]
		}

		rendered := formatTableRow(row, widths, columns, false, m.offset+visibleIndex == m.cursor)
		lines = append(lines, rendered)
		if visibleIndex < end-m.offset-1 {
			lines = append(lines, rowDivider(width))
		}
	}
	return strings.Join(lines, "\n")
}

func (m model) tableRowsForWidths(columns []ui.ColumnDef) [][]string {
	rows := make([][]string, 0, len(m.filtered))
	for visibleIndex, filteredIndex := range m.filtered {
		item := m.items[filteredIndex]
		row := make([]string, 0, len(columns))
		row = append(row, ui.FormatRowNum(visibleIndex+1, len(m.filtered)))
		row = append(row, item.Columns...)
		for len(row) < len(columns) {
			row = append(row, "")
		}
		if len(row) > len(columns) {
			row = row[:len(columns)]
		}
		rows = append(rows, row)
	}
	return rows
}

func (m model) tableColumns() []ui.ColumnDef {
	if len(m.opts.Columns) > 0 {
		return m.opts.Columns
	}
	if len(m.opts.Headers) == 0 {
		return nil
	}
	return fallbackTableColumns(len(m.opts.Headers))
}

func (m model) tableHeaders(columns []ui.ColumnDef) []string {
	headers := make([]string, len(columns))
	for i, col := range columns {
		headers[i] = col.Name
	}
	for i, header := range m.opts.Headers {
		if i < len(headers) {
			headers[i] = header
		}
	}
	return headers
}

func (m model) listHeight() int {
	bodyHeight := m.height - 4
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	if len(m.tableColumns()) > 0 {
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

func (m *model) toggleCurrent() {
	if !m.opts.MultiSelect || len(m.filtered) == 0 {
		return
	}
	index := m.filtered[m.cursor]
	key := m.selectionKey(index)
	if m.selectedKeys[key] {
		delete(m.selectedKeys, key)
		return
	}
	m.selectedKeys[key] = true
}

func (m model) isSelected(index int) bool {
	return m.opts.MultiSelect && m.selectedKeys[m.selectionKey(index)]
}

func (m model) selectedCount() int {
	if !m.opts.MultiSelect {
		return 0
	}
	return len(m.selectedKeys)
}

func (m model) selectionKey(index int) string {
	if index >= 0 && index < len(m.items) && strings.TrimSpace(m.items[index].ID) != "" {
		return "id:" + m.items[index].ID
	}
	return fmt.Sprintf("index:%d", index)
}

func (m model) selections() []Selection {
	if m.action == ActionBack {
		return []Selection{{Action: ActionBack}}
	}
	if len(m.filtered) == 0 {
		return nil
	}
	if !m.opts.MultiSelect || len(m.selectedKeys) == 0 {
		return []Selection{{Item: m.items[m.filtered[m.cursor]], Action: m.action}}
	}
	selections := make([]Selection, 0, len(m.selectedKeys))
	for index, item := range m.items {
		if m.selectedKeys[m.selectionKey(index)] {
			selections = append(selections, Selection{Item: item, Action: m.action})
		}
	}
	return selections
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

func fallbackTableColumns(count int) []ui.ColumnDef {
	if count <= 0 {
		return nil
	}
	columns := make([]ui.ColumnDef, count)
	columns[0] = ui.ColumnDef{
		Name:     "num",
		MinWidth: 4,
		MaxWidth: 6,
		Align:    ui.AlignRight,
		HasStyle: true,
		Style:    ui.Muted,
	}
	for i := 1; i < count; i++ {
		columns[i] = ui.ColumnDef{
			Name:       fmt.Sprintf("column:%d", i),
			WidthRatio: 1,
			MinWidth:   6,
			Align:      ui.AlignLeft,
		}
	}
	return columns
}

func formatTableRow(cells []string, widths []int, columns []ui.ColumnDef, header bool, selected bool) string {
	parts := make([]string, 0, len(widths))
	for i, width := range widths {
		cell := ""
		if i < len(cells) {
			cell = cells[i]
		}
		cell = truncate(cell, width)
		style := tableCellStyle(width, columns, i, header, selected)
		parts = append(parts, style.Render(cell))
	}
	return strings.Join(parts, "  ")
}

func tableCellStyle(width int, columns []ui.ColumnDef, index int, header bool, selected bool) lipgloss.Style {
	style := lipgloss.NewStyle().Width(width)
	if index < len(columns) {
		switch columns[index].Align {
		case ui.AlignRight:
			style = style.Align(lipgloss.Right)
		case ui.AlignCenter:
			style = style.Align(lipgloss.Center)
		default:
			style = style.Align(lipgloss.Left)
		}
		if header {
			style = mutedStyle.Bold(true).Width(width)
		} else if columns[index].HasStyle {
			style = columns[index].Style.Width(width)
		}
	}
	if selected && !header {
		style = style.Bold(true)
	}
	return style
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
