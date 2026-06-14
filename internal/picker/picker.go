package picker

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// Item is a selectable entry in a Raven-owned interactive picker.
type Item struct {
	ID       string
	Label    string
	Detail   string
	Location string
	FilePath string
	Line     int
	Preview  string
}

// Options controls picker copy and layout.
type Options struct {
	Title  string
	Prompt string
}

// Selection is the item selected by the user.
type Selection struct {
	Item Item
}

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
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyEsc:
			return m, tea.Quit
		case tea.KeyEnter:
			if len(m.filtered) > 0 {
				m.selected = true
			}
			return m, tea.Quit
		case tea.KeyUp:
			m.moveCursor(-1)
		case tea.KeyDown:
			m.moveCursor(1)
		case tea.KeyPgUp:
			m.moveCursor(-m.listHeight())
		case tea.KeyPgDown:
			m.moveCursor(m.listHeight())
		case tea.KeyBackspace, tea.KeyDelete:
			if m.query != "" {
				m.query = dropLastRune(m.query)
				m.applyFilter()
			}
		case tea.KeyRunes:
			switch msg.String() {
			case "j":
				m.moveCursor(1)
			case "k":
				m.moveCursor(-1)
			case "q":
				return m, tea.Quit
			default:
				m.query += msg.String()
				m.applyFilter()
			}
		}
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
	filter := mutedStyle.Render(fmt.Sprintf("%s: ", prompt)) + m.query
	help := mutedStyle.Render("enter: open  esc/q: cancel  ↑/↓: move  type: filter")

	list := m.renderList()
	preview := m.renderPreview()

	bodyHeight := m.height - 4
	if bodyHeight < 8 {
		bodyHeight = 8
	}
	listWidth := m.width / 2
	if listWidth < 40 {
		listWidth = m.width
	}
	previewWidth := m.width - listWidth - 2

	var body string
	if previewWidth >= 30 {
		body = lipgloss.JoinHorizontal(lipgloss.Top,
			lipgloss.NewStyle().Width(listWidth).Height(bodyHeight).Render(list),
			lipgloss.NewStyle().Width(previewWidth).Height(bodyHeight).Render(preview),
		)
	} else {
		body = lipgloss.NewStyle().Height(bodyHeight).Render(list)
	}

	return strings.Join([]string{header, filter, body, help}, "\n")
}

func (m model) renderList() string {
	if len(m.filtered) == 0 {
		return mutedStyle.Render("No matches")
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
	}
	return strings.Join(lines, "\n")
}

func (m model) renderPreview() string {
	if len(m.filtered) == 0 {
		return ""
	}

	item := m.items[m.filtered[m.cursor]]
	title := item.Label
	if item.Location != "" {
		title += " " + mutedStyle.Render("("+item.Location+")")
	}

	content := strings.TrimRight(item.Preview, "\n")
	if content == "" {
		content = mutedStyle.Render("No preview available")
	}

	height := m.listHeight() - 2
	if height < 1 {
		height = 1
	}
	lines := strings.Split(content, "\n")
	if len(lines) > height {
		lines = lines[:height]
	}

	return titleStyle.Render(title) + "\n" + strings.Join(lines, "\n")
}

func (m model) listHeight() int {
	height := m.height - 5
	if height < 5 {
		return 5
	}
	return height
}

func (m *model) applyFilter() {
	m.filtered = m.filtered[:0]
	for i, item := range m.items {
		if matches(item, m.query) {
			m.filtered = append(m.filtered, i)
		}
	}
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

func matches(item Item, query string) bool {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		item.Label,
		item.Detail,
		item.Location,
		item.ID,
		item.FilePath,
	}, " "))
	return fuzzyContains(haystack, query)
}

func fuzzyContains(haystack, query string) bool {
	pos := 0
	for _, r := range query {
		found := false
		for pos < len(haystack) {
			if rune(haystack[pos]) == r {
				found = true
				pos++
				break
			}
			pos++
		}
		if !found {
			return false
		}
	}
	return true
}

func dropLastRune(s string) string {
	if s == "" {
		return ""
	}
	runes := []rune(s)
	return string(runes[:len(runes)-1])
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true)
	selectedStyle = lipgloss.NewStyle().Bold(true)
	mutedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
)
