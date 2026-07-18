package cli

import (
	"errors"
	"slices"
	"testing"

	"github.com/aidanlsb/raven/internal/picker"
)

func TestCleanPickerPrompt(t *testing.T) {
	if got := cleanPickerPrompt("open> "); got != "open" {
		t.Fatalf("cleanPickerPrompt(%q) = %q, want open", "open> ", got)
	}
	if got := cleanPickerPrompt("docs/section"); got != "docs/section" {
		t.Fatalf("cleanPickerPrompt(%q) = %q, want docs/section", "docs/section", got)
	}
}

func TestSelectorBrowseRejectsSelectionWithoutFilePath(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() { ravenRunPicker = prevRun })

	ravenRunPicker = func(_ []picker.Item, _ picker.Options) (picker.Selection, bool, error) {
		return picker.Selection{Item: picker.Item{ID: "note/one"}}, true, nil
	}

	item, ok, err := cliSelector.browse(browsePickerOptions{
		Title:                  "Query results",
		Items:                  []picker.Item{{ID: "note/one"}},
		MissingFilePathMessage: "selected query result has no file path",
	})
	if err == nil {
		t.Fatalf("expected error when selection has no file path")
	}
	if ok {
		t.Fatalf("expected ok=false when selection has no file path")
	}
	if item.ID != "" {
		t.Fatalf("expected zero item, got %#v", item)
	}
}

func TestSelectorBrowseCancelReturnsNoError(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() { ravenRunPicker = prevRun })

	ravenRunPicker = func(_ []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if opts.Prompt != "filter" {
			t.Fatalf("prompt = %q, want filter", opts.Prompt)
		}
		return picker.Selection{}, false, nil
	}

	_, ok, err := cliSelector.browse(browsePickerOptions{
		Title: "Query results",
		Items: []picker.Item{{ID: "note/one", FilePath: "note/one.md"}},
	})
	if err != nil {
		t.Fatalf("browse() cancel error = %v", err)
	}
	if ok {
		t.Fatalf("expected ok=false on cancel")
	}
}

func TestSelectorPipeItemsSingleNormalizesToSlice(t *testing.T) {
	prevRun := ravenRunPicker
	prevMulti := ravenRunPickerMulti
	t.Cleanup(func() {
		ravenRunPicker = prevRun
		ravenRunPickerMulti = prevMulti
	})

	ravenRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if opts.Title != "Pick items" {
			t.Fatalf("title = %q, want Pick items", opts.Title)
		}
		if !slices.Equal(opts.Headers, []string{"#", "content", "id", "location"}) {
			t.Fatalf("headers = %#v", opts.Headers)
		}
		return picker.Selection{Item: items[0]}, true, nil
	}
	ravenRunPickerMulti = func([]picker.Item, picker.Options) ([]picker.Selection, bool, error) {
		t.Fatalf("multi runner should not be called for single select")
		return nil, false, nil
	}

	items := []picker.Item{{ID: "project/raven"}}
	selections, ok, err := cliSelector.pipeItems(items, false, nil)
	if err != nil {
		t.Fatalf("pipeItems() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected selection")
	}
	if len(selections) != 1 || selections[0].Item.ID != "project/raven" {
		t.Fatalf("selections = %#v, want single project/raven", selections)
	}
}

func TestSelectorPipeItemsMultiDelegatesToMultiRunner(t *testing.T) {
	prevRun := ravenRunPicker
	prevMulti := ravenRunPickerMulti
	t.Cleanup(func() {
		ravenRunPicker = prevRun
		ravenRunPickerMulti = prevMulti
	})

	ravenRunPicker = func([]picker.Item, picker.Options) (picker.Selection, bool, error) {
		t.Fatalf("single runner should not be called for multi select")
		return picker.Selection{}, false, nil
	}
	ravenRunPickerMulti = func(items []picker.Item, _ picker.Options) ([]picker.Selection, bool, error) {
		return []picker.Selection{{Item: items[0]}, {Item: items[1]}}, true, nil
	}

	items := []picker.Item{{ID: "a"}, {ID: "b"}}
	selections, ok, err := cliSelector.pipeItems(items, true, nil)
	if err != nil || !ok {
		t.Fatalf("pipeItems() multi = (%v, %v)", ok, err)
	}
	if len(selections) != 2 {
		t.Fatalf("selections = %#v, want 2", selections)
	}
}

func TestSelectorPipeItemsPropagatesRunnerError(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() { ravenRunPicker = prevRun })

	sentinel := errors.New("boom")
	ravenRunPicker = func([]picker.Item, picker.Options) (picker.Selection, bool, error) {
		return picker.Selection{}, false, sentinel
	}

	_, ok, err := cliSelector.pipeItems([]picker.Item{{ID: "a"}}, false, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("err = %v, want sentinel", err)
	}
	if ok {
		t.Fatalf("expected ok=false on runner error")
	}
}
