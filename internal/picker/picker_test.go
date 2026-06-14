package picker

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/aidanlsb/raven/internal/ui"
)

func TestRankItemsUsesSearchText(t *testing.T) {
	items := []Item{
		{ID: "visible", Label: "Visible label"},
		{ID: "hidden", Label: "Other label", SearchText: "hidden raven metadata"},
	}

	filtered := rankItems(items, "raven")
	if len(filtered) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(filtered))
	}
	if got := items[filtered[0]].ID; got != "hidden" {
		t.Fatalf("filtered item = %q, want hidden", got)
	}
}

func TestRankItemsPreservesEmptyQueryOrder(t *testing.T) {
	items := []Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}

	filtered := rankItems(items, "")
	if strings.Join([]string{items[filtered[0]].ID, items[filtered[1]].ID}, ",") != "one,two" {
		t.Fatalf("filtered order = %#v, want original order", filtered)
	}
}

func TestRankItemsSortsByMatchQuality(t *testing.T) {
	items := []Item{
		{ID: "weak", SearchText: "really arbitrary verbose entry near"},
		{ID: "strong", SearchText: "raven"},
	}

	filtered := rankItems(items, "raven")
	if len(filtered) != 2 {
		t.Fatalf("filtered count = %d, want 2", len(filtered))
	}
	if got := items[filtered[0]].ID; got != "strong" {
		t.Fatalf("top ranked item = %q, want strong", got)
	}
}

func TestModelFiltersAndSelects(t *testing.T) {
	m := newModel([]Item{
		{ID: "issue/one", Label: "First issue"},
		{ID: "issue/two", Label: "Second issue"},
	}, Options{Title: "Issues"})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("second")})
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

func TestInsertModeFiltersPrintableNavigationLetters(t *testing.T) {
	m := newModel([]Item{
		{ID: "issue/jkqg", Label: "jkqg shortcuts should filter"},
		{ID: "issue/other", Label: "Other issue"},
	}, Options{Title: "Issues"})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(model)
	for _, r := range "jkqg" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
		m = updated.(model)
	}

	if m.query != "jkqg" {
		t.Fatalf("query = %q, want jkqg", m.query)
	}
	if len(m.filtered) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(m.filtered))
	}
	if got := m.items[m.filtered[0]].ID; got != "issue/jkqg" {
		t.Fatalf("filtered item = %q, want issue/jkqg", got)
	}
}

func TestNormalModeVimKeysNavigateAndQuit(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}, Options{})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if cmd != nil {
		t.Fatalf("esc in normal mode should not quit")
	}
	if m.mode != normalMode {
		t.Fatalf("mode after esc = %v, want normal", m.mode)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(model)
	if m.cursor != 1 {
		t.Fatalf("cursor after j = %d, want 1", m.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(model)
	if m.cursor != 0 {
		t.Fatalf("cursor after k = %d, want 0", m.cursor)
	}

	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("q in normal mode should quit")
	}
	if m.selected {
		t.Fatalf("q should quit without selecting")
	}
}

func TestNormalModeVimTopBottomNavigation(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
		{ID: "three", Label: "Three"},
	}, Options{})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("G")})
	m = updated.(model)
	if m.cursor != 2 {
		t.Fatalf("cursor after G = %d, want 2", m.cursor)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated.(model)
	if m.cursor != 2 {
		t.Fatalf("cursor after first g = %d, want unchanged 2", m.cursor)
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("g")})
	m = updated.(model)
	if m.cursor != 0 {
		t.Fatalf("cursor after gg = %d, want 0", m.cursor)
	}
}

func TestNormalModeForwardAndBackActions(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}, Options{AllowForward: true, AllowBack: true})

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("l should quit with forward action")
	}
	if !m.selected || m.action != ActionForward {
		t.Fatalf("selected=%v action=%q, want forward selection", m.selected, m.action)
	}
	selections := m.selections()
	if len(selections) != 1 || selections[0].Action != ActionForward || selections[0].Item.ID != "one" {
		t.Fatalf("selections = %#v, want forward selection of first item", selections)
	}

	m = newModel([]Item{{ID: "one", Label: "One"}}, Options{AllowBack: true})
	updated, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("H")})
	m = updated.(model)
	if cmd == nil {
		t.Fatalf("H should quit with back action")
	}
	selections = m.selections()
	if len(selections) != 1 || selections[0].Action != ActionBack {
		t.Fatalf("selections = %#v, want back action", selections)
	}
}

func TestViewRendersShortcutGutterWhenConfigured(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
	}, Options{
		Shortcuts: []ShortcutTip{
			{Key: "h", Description: "back"},
			{Key: "l", Description: "open"},
		},
	})
	m.width = 100

	view := m.View()
	for _, want := range []string{"shortcuts", "h         back", "l         open"} {
		if !strings.Contains(view, want) {
			t.Fatalf("view missing %q:\n%s", want, view)
		}
	}
}

func TestMultiSelectTogglesCurrentItem(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}, Options{MultiSelect: true})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(model)
	if !m.isSelected(0) {
		t.Fatalf("first item should be selected")
	}
	if got := m.selectedCount(); got != 1 {
		t.Fatalf("selected count = %d, want 1", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(model)
	selections := m.selections()
	if len(selections) != 2 {
		t.Fatalf("selection count = %d, want 2", len(selections))
	}
	if selections[0].Item.ID != "one" || selections[1].Item.ID != "two" {
		t.Fatalf("selections = %#v, want original item order", selections)
	}
}

func TestMultiSelectEnterFallsBackToCurrentItem(t *testing.T) {
	m := newModel([]Item{
		{ID: "one", Label: "One"},
		{ID: "two", Label: "Two"},
	}, Options{MultiSelect: true})
	m.moveCursor(1)

	selections := m.selections()
	if len(selections) != 1 {
		t.Fatalf("selection count = %d, want 1", len(selections))
	}
	if selections[0].Item.ID != "two" {
		t.Fatalf("selected item = %q, want two", selections[0].Item.ID)
	}
}

func TestMultiSelectPreservesSelectionAcrossFiltering(t *testing.T) {
	m := newModel([]Item{
		{ID: "alpha", Label: "Alpha"},
		{ID: "beta", Label: "Beta"},
	}, Options{MultiSelect: true})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeySpace})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("beta")})
	m = updated.(model)

	if len(m.filtered) != 1 {
		t.Fatalf("filtered count = %d, want 1", len(m.filtered))
	}
	if !m.isSelected(0) {
		t.Fatalf("alpha should remain selected while filtered out")
	}
	selections := m.selections()
	if len(selections) != 1 || selections[0].Item.ID != "alpha" {
		t.Fatalf("selections = %#v, want alpha", selections)
	}
}

func TestInsertModeEscReturnsToNormal(t *testing.T) {
	m := newModel([]Item{{ID: "one", Label: "One"}}, Options{})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(model)
	if m.mode != insertMode {
		t.Fatalf("mode after / = %v, want insert", m.mode)
	}
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(model)
	if cmd != nil {
		t.Fatalf("esc in insert mode should not quit")
	}
	if m.mode != normalMode {
		t.Fatalf("mode after esc = %v, want normal", m.mode)
	}
}

func TestInsertModeControlEditing(t *testing.T) {
	m := newModel([]Item{
		{ID: "alpha", Label: "alpha beta"},
		{ID: "other", Label: "Other issue"},
	}, Options{})

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("i")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("alpha beta")})
	m = updated.(model)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = updated.(model)
	if m.query != "alpha " {
		t.Fatalf("query after ctrl-w = %q, want %q", m.query, "alpha ")
	}
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = updated.(model)
	if m.query != "" {
		t.Fatalf("query after ctrl-u = %q, want empty", m.query)
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

func TestRenderTableListIncludesHeadersColumnsAndDividers(t *testing.T) {
	m := newModel([]Item{
		{
			ID:       "issue/one",
			Label:    "Issue One",
			Columns:  []string{"Issue One", "suggestion", "raven", "open", "type/issue/one.md:1"},
			Location: "type/issue/one.md:1",
		},
		{
			ID:       "issue/two",
			Label:    "Issue Two",
			Columns:  []string{"Issue Two", "-", "raven", "open", "type/issue/two.md:1"},
			Location: "type/issue/two.md:1",
		},
	}, Options{
		Headers: []string{"#", "title", "category", "project", "status", "location"},
		Columns: ui.ObjectLayout([]string{"category", "project", "status"}),
	})

	out := m.renderList(80)
	for _, want := range []string{"title", "category", "project", "status", "» 1", "Issue One", "suggestion", "type/issue/one.md:1", "─"} {
		if !strings.Contains(out, want) {
			t.Fatalf("rendered table missing %q:\n%s", want, out)
		}
	}
}

func TestFormatTableRowBoldsSelectedContentCells(t *testing.T) {
	columns := fallbackTableColumns(2)
	if !tableCellStyle(20, columns, 1, false, true).GetBold() {
		t.Fatalf("selected content cell should be bold")
	}
	if tableCellStyle(20, columns, 1, false, false).GetBold() {
		t.Fatalf("unselected content cell should not be bold")
	}
	if !tableCellStyle(20, columns, 1, true, false).GetBold() {
		t.Fatalf("header content cell should remain bold")
	}
}

func TestRenderTableListFitsBodyHeight(t *testing.T) {
	items := make([]Item, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, Item{
			ID:       "todo",
			Label:    "Todo item",
			Columns:  []string{"Todo item with context", "@todo", "type/project/raven.md:1"},
			Location: "type/project/raven.md:1",
		})
	}
	m := newModel(items, Options{
		Headers: []string{"#", "content", "trait", "location"},
	})
	m.height = 12

	out := m.renderList(100)
	bodyHeight := m.height - 4
	if got := len(strings.Split(out, "\n")); got > bodyHeight {
		t.Fatalf("rendered lines = %d, want <= body height %d:\n%s", got, bodyHeight, out)
	}
}

func TestTableWidthsFitAvailableWidth(t *testing.T) {
	headers := []string{"#", "title", "category", "project", "status", "location"}
	widths := ui.CalculateColumnWidths(fallbackTableColumns(len(headers)), 80)
	total := 0
	for _, width := range widths {
		total += width
	}
	total += 2 * (len(widths) - 1)
	if total > 80 {
		t.Fatalf("table width = %d, want <= 80 (widths=%v)", total, widths)
	}
}
