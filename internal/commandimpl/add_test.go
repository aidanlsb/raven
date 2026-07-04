package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestHandleAddBulkAppendsWithinSection(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks

- existing task

## Notes

some notes
`)
	reindexForEditTest(t, v.Path)

	args := map[string]any{
		"text":       "- new task",
		"stdin":      true,
		"object_ids": []interface{}{"note/example#tasks"},
	}

	preview := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      args,
	})
	if !preview.OK {
		t.Fatalf("HandleAdd preview failed: %#v", preview.Error)
	}
	previewData, _ := preview.Data.(map[string]interface{})
	items, _ := previewData["items"].([]canonicalBulkPreviewItem)
	if len(items) != 1 || items[0].ID != "note/example#tasks" {
		t.Fatalf("preview items = %#v, want one item for note/example#tasks", items)
	}
	if warnings, ok := previewData["warnings"].([]commandexec.Warning); ok && len(warnings) > 0 {
		t.Fatalf("preview warnings = %#v, want none", warnings)
	}

	apply := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args:      args,
	})
	if !apply.OK {
		t.Fatalf("HandleAdd apply failed: %#v", apply.Error)
	}
	for _, w := range apply.Warnings {
		if w.Code == warnSectionSkipped {
			t.Fatalf("unexpected SECTION_SKIPPED warning: %#v", w)
		}
	}

	content := v.ReadFile("note/example.md")
	tasksIdx := strings.Index(content, "## Tasks")
	notesIdx := strings.Index(content, "## Notes")
	newIdx := strings.Index(content, "- new task")
	if newIdx == -1 {
		t.Fatalf("appended line not found in file:\n%s", content)
	}
	if !(tasksIdx < newIdx && newIdx < notesIdx) {
		t.Fatalf("appended line not inside Tasks section (tasks=%d new=%d notes=%d):\n%s", tasksIdx, newIdx, notesIdx, content)
	}
}

func TestHandleAddBulkMixedFileAndSectionIDs(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks

- existing task
`)
	reindexForEditTest(t, v.Path)

	apply := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args: map[string]any{
			"text":       "- mixed line",
			"stdin":      true,
			"object_ids": []interface{}{"note/example", "note/example#tasks"},
		},
	})
	if !apply.OK {
		t.Fatalf("HandleAdd apply failed: %#v", apply.Error)
	}
	data, _ := apply.Data.(map[string]interface{})
	if added, _ := data["added"].(int); added != 2 {
		t.Fatalf("added = %v, want 2 (data = %#v)", data["added"], data)
	}
	if got := strings.Count(v.ReadFile("note/example.md"), "- mixed line"); got != 2 {
		t.Fatalf("expected 2 appended lines, got %d:\n%s", got, v.ReadFile("note/example.md"))
	}
}

func TestHandleSetBulkWarnsOnSectionIDs(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks

- existing task
`)
	reindexForEditTest(t, v.Path)

	result := HandleSet(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"stdin":      true,
			"object_ids": []interface{}{"note/example", "note/example#tasks"},
			"fields":     []string{"title=Renamed"},
		},
	})
	if !result.OK {
		t.Fatalf("HandleSet preview failed: %#v", result.Error)
	}
	data, _ := result.Data.(map[string]interface{})
	warnings, _ := data["warnings"].([]commandexec.Warning)
	if len(warnings) != 1 || warnings[0].Code != warnSectionSkipped {
		t.Fatalf("warnings = %#v, want one SECTION_SKIPPED warning", warnings)
	}
	items, _ := data["items"].([]canonicalBulkPreviewItem)
	if len(items) != 1 || items[0].ID != "note/example" {
		t.Fatalf("preview items = %#v, want only note/example", items)
	}
}
