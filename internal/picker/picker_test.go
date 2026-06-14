package picker

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMatchesUsesLabelDetailLocationAndID(t *testing.T) {
	item := Item{
		ID:       "issue/query-table-fields",
		Label:    "Check if queries return fields",
		Detail:   "status=open project=raven",
		Location: "type/issue/query-table-fields.md:1",
		FilePath: "type/issue/query-table-fields.md",
	}

	for _, query := range []string{"queries", "status", "raven", "issue/query", "qtf"} {
		if !matches(item, query) {
			t.Fatalf("matches(%q) = false, want true", query)
		}
	}
	if matches(item, "zzzz") {
		t.Fatalf("matches unrelated query = true, want false")
	}
}

func TestModelFiltersAndSelects(t *testing.T) {
	m := newModel([]Item{
		{ID: "issue/one", Label: "First issue"},
		{ID: "issue/two", Label: "Second issue"},
	}, Options{Title: "Issues"})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second")})
	m = updated.(model)
	if len(m.filtered) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(m.filtered))
	}
	if got := m.items[m.filtered[0]].ID; got != "issue/two" {
		t.Fatalf("filtered item = %q, want issue/two", got)
	}

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("enter should return quit command")
	}
	if !m.selected {
		t.Fatalf("selected = false, want true")
	}
}

func TestModelNavigationClamps(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}, Options{})

	m.moveCursor(10)
	if m.cursor != 1 {
		t.Fatalf("cursor after large down = %d, want 1", m.cursor)
	}
	m.moveCursor(-10)
	if m.cursor != 0 {
		t.Fatalf("cursor after large up = %d, want 0", m.cursor)
	}
}
