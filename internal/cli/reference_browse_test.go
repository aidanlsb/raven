package cli

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
)

func TestBrowseItemsForBacklinkResultsUseColumnsAndSearchText(t *testing.T) {
	line := 12
	display := "planning note"
	items := browseItemsForBacklinkResults([]model.Reference{
		{
			SourceID:    "note/planning",
			SourceType:  "object",
			TargetRaw:   "project/raven",
			FilePath:    "note/planning.md",
			Line:        &line,
			DisplayText: &display,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	wantColumns := []string{"planning note", "note/planning.md:12"}
	if !slices.Equal(item.Columns, wantColumns) {
		t.Fatalf("columns = %#v, want %#v", item.Columns, wantColumns)
	}
	if item.FilePath != "note/planning.md" || item.Line != 12 {
		t.Fatalf("location = %q:%d, want note/planning.md:12", item.FilePath, item.Line)
	}
	for _, want := range []string{"note/planning", "project/raven", "planning note", "note/planning.md:12"} {
		if !strings.Contains(item.SearchText, want) {
			t.Fatalf("search text missing %q: %q", want, item.SearchText)
		}
	}
}

func TestBrowseItemsForOutlinkResultsUseDisplayTextWithTarget(t *testing.T) {
	line := 7
	display := "Raven"
	items := browseItemsForOutlinkResults([]model.Reference{
		{
			SourceID:    "note/planning",
			SourceType:  "object",
			TargetRaw:   "project/raven",
			FilePath:    "note/planning.md",
			Line:        &line,
			DisplayText: &display,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	wantColumns := []string{"Raven (project/raven)", "note/planning.md:7"}
	if !slices.Equal(item.Columns, wantColumns) {
		t.Fatalf("columns = %#v, want %#v", item.Columns, wantColumns)
	}
	for _, want := range []string{"note/planning", "project/raven", "Raven", "note/planning.md:7"} {
		if !strings.Contains(item.SearchText, want) {
			t.Fatalf("search text missing %q: %q", want, item.SearchText)
		}
	}
}

func TestBrowseReferencesUsesRavenPickerLayout(t *testing.T) {
	prevRun := ravenRunPicker
	t.Cleanup(func() {
		ravenRunPicker = prevRun
	})

	items := []picker.Item{{ID: "one", FilePath: "note/one.md", Line: 3}}
	ravenRunPicker = func(gotItems []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		if !reflect.DeepEqual(gotItems, items) {
			t.Fatalf("items = %#v, want %#v", gotItems, items)
		}
		if opts.Title != "Backlinks to project/raven" {
			t.Fatalf("title = %q, want Backlinks to project/raven", opts.Title)
		}
		if opts.Prompt != "filter" {
			t.Fatalf("prompt = %q, want filter", opts.Prompt)
		}
		if !slices.Equal(opts.Headers, []string{"#", "content", "location"}) {
			t.Fatalf("headers = %#v", opts.Headers)
		}
		return picker.Selection{Item: gotItems[0]}, true, nil
	}

	item, ok, err := browseReferences("Backlinks to project/raven", items)
	if err != nil {
		t.Fatalf("browseReferences() error = %v", err)
	}
	if !ok {
		t.Fatalf("expected selected item")
	}
	if item.FilePath != "note/one.md" || item.Line != 3 {
		t.Fatalf("selected item = %#v", item)
	}
}

func TestValidateReferenceBrowseFlag(t *testing.T) {
	prevJSON := jsonOutput
	prevStdinTTY := interactiveStdinIsTerminal
	prevStdoutTTY := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		interactiveStdinIsTerminal = prevStdinTTY
		interactiveStdoutIsTerminal = prevStdoutTTY
	})

	cmd := &cobra.Command{}
	cmd.Flags().Bool("browse", false, "")
	if err := cmd.Flags().Set("browse", "true"); err != nil {
		t.Fatalf("set browse flag: %v", err)
	}

	jsonOutput = true
	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }
	if handled, err := validateReferenceBrowseFlag(cmd); !handled || err != nil {
		t.Fatalf("expected handled --json/--browse conflict, got handled=%v err=%v", handled, err)
	}

	jsonOutput = false
	interactiveStdinIsTerminal = func() bool { return false }
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err == nil {
		t.Fatalf("expected non-interactive browse error")
	}

	interactiveStdinIsTerminal = func() bool { return true }
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err != nil {
		t.Fatalf("validateReferenceBrowseFlag() handled=%v error=%v", handled, err)
	}
}
