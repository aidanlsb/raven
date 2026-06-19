package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestBrowseItemsForObjectResultsUseNameFieldAndDetails(t *testing.T) {
	sch := schema.New()
	sch.Types["issue"] = &schema.TypeDefinition{
		NameField: "title",
		Fields: map[string]*schema.FieldDefinition{
			"title":   {Type: schema.FieldTypeString},
			"project": {Type: schema.FieldTypeRef},
			"status":  {Type: schema.FieldTypeString},
		},
	}

	items := browseItemsForObjectResults([]model.Object{
		{
			ID:        "issue/check-if-queries-return-the-fields-in-the-printed-table",
			Type:      "issue",
			FilePath:  "type/issue/check-if-queries-return-the-fields-in-the-printed-table.md",
			LineStart: 1,
			Fields: map[string]interface{}{
				"title":   "Check if queries return the fields in the printed table",
				"project": "project/raven",
				"status":  "open",
			},
		},
	}, sch)

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	if item.Label != "Check if queries return the fields in the printed table" {
		t.Fatalf("label = %q, want title field", item.Label)
	}
	wantColumns := []string{
		"Check if queries return the fields in the printed table",
		"raven",
		"open",
		"type/issue/check-if-queries-return-the-fields-in-the-printed-table.md:1",
	}
	if strings.Join(item.Columns, "|") != strings.Join(wantColumns, "|") {
		t.Fatalf("columns = %#v, want %#v", item.Columns, wantColumns)
	}
	if !strings.Contains(item.Detail, "project=raven") || !strings.Contains(item.Detail, "status=open") {
		t.Fatalf("detail = %q, want field summary", item.Detail)
	}
	if strings.Contains(item.Detail, item.ID) {
		t.Fatalf("detail should not repeat object id, got %q", item.Detail)
	}
	for _, want := range []string{"issue/check-if-queries", "Check if queries return", "project=raven", "type/issue/check-if-queries-return-the-fields-in-the-printed-table.md:1"} {
		if !strings.Contains(item.SearchText, want) {
			t.Fatalf("search text missing %q: %q", want, item.SearchText)
		}
	}
}

func TestBrowseItemsForTraitResultsUseColumnsAndSearchText(t *testing.T) {
	value := "done"
	items := browseItemsForTraitResults([]model.Trait{
		{
			ID:             "type/project/raven.md:trait:1",
			TraitType:      "todo",
			Value:          &value,
			Content:        "Polish the Raven query picker so trait result rows have enough surrounding context",
			FilePath:       "type/project/raven.md",
			Line:           42,
			ParentObjectID: "project/raven",
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	wantColumns := []string{
		"Polish the Raven query picker so trait result rows have enough surrounding context",
		"@todo(done)",
		"type/project/raven.md:42",
	}
	if strings.Join(item.Columns, "|") != strings.Join(wantColumns, "|") {
		t.Fatalf("columns = %#v, want %#v", item.Columns, wantColumns)
	}
	for _, want := range []string{"todo", "done", "surrounding context", "project/raven", "type/project/raven.md:42"} {
		if !strings.Contains(item.SearchText, want) {
			t.Fatalf("search text missing %q: %q", want, item.SearchText)
		}
	}
}

func TestBrowseItemsForSectionResultsUseColumnsAndSearchText(t *testing.T) {
	parentID := "page/raven#query-picker"
	items := browseItemsForSectionResults([]model.Section{
		{
			ID:              "page/raven#section-query-results",
			FileObjectID:    "page/raven",
			FilePath:        "page/raven.md",
			Slug:            "section-query-results",
			Title:           "Section query results",
			Level:           2,
			LineStart:       17,
			ParentSectionID: &parentID,
		},
	})

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	wantColumns := []string{
		"Section query results",
		"h2 #section-query-results",
		"page/raven.md:17",
	}
	if strings.Join(item.Columns, "|") != strings.Join(wantColumns, "|") {
		t.Fatalf("columns = %#v, want %#v", item.Columns, wantColumns)
	}
	for _, want := range []string{"page/raven#section-query-results", "Section query results", "h2 #section-query-results", "page/raven.md:17", parentID} {
		if !strings.Contains(item.SearchText, want) {
			t.Fatalf("search text missing %q: %q", want, item.SearchText)
		}
	}
}

func TestBrowseQueryResultsUsesSharedPickerAndEditorHandoff(t *testing.T) {
	t.Setenv("EDITOR", "")

	prevRun := ravenRunPicker
	prevVaultPath := resolvedVaultPath
	prevCfg := cfg
	t.Cleanup(func() {
		ravenRunPicker = prevRun
		resolvedVaultPath = prevVaultPath
		cfg = prevCfg
	})

	vaultPath := t.TempDir()
	relPath := filepath.Join("note", "one.md")
	absPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(absPath), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(absPath, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatalf("write note: %v", err)
	}

	resolvedVaultPath = vaultPath
	cfg = &config.Config{}
	called := false
	ravenRunPicker = func(items []picker.Item, opts picker.Options) (picker.Selection, bool, error) {
		called = true
		if opts.Title != "Query results" {
			t.Fatalf("title = %q, want Query results", opts.Title)
		}
		if opts.Prompt != "filter" {
			t.Fatalf("prompt = %q, want filter", opts.Prompt)
		}
		if len(items) != 1 || items[0].FilePath != relPath {
			t.Fatalf("items = %#v", items)
		}
		if opts.Preview == nil {
			t.Fatalf("expected preview function")
		}
		return picker.Selection{Item: items[0]}, true, nil
	}

	out := captureStdout(t, func() {
		if err := browseQueryResults([]picker.Item{{ID: "note/one", FilePath: relPath, Line: 2}}, []string{"#", "title", "location"}, nil); err != nil {
			t.Fatalf("browseQueryResults() error = %v", err)
		}
	})
	if !called {
		t.Fatalf("expected picker to run")
	}
	if !strings.Contains(out, relPath+":2") {
		t.Fatalf("expected editor handoff output to include selected line, got:\n%s", out)
	}
}
