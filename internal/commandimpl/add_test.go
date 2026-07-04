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

func TestHandleAddCreateHeading(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks

- existing task
`)
	reindexForEditTest(t, v.Path)

	// Without --create-heading, a missing heading is an error.
	missing := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"text":    "- log entry",
			"to":      "note/example",
			"heading": "### Log",
		},
	})
	if missing.OK {
		t.Fatalf("expected REF_NOT_FOUND without --create-heading, got success: %#v", missing.Data)
	}

	result := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"text":           "- log entry",
			"to":             "note/example",
			"heading":        "### Log",
			"create-heading": true,
		},
	})
	if !result.OK {
		t.Fatalf("HandleAdd with create-heading failed: %#v", result.Error)
	}
	data, _ := result.Data.(map[string]interface{})
	if created, _ := data["created_heading"].(bool); !created {
		t.Fatalf("created_heading = %#v, want true (data = %#v)", data["created_heading"], data)
	}
	if section, _ := data["section"].(string); section != "note/example#log" {
		t.Fatalf("section = %#v, want note/example#log", data["section"])
	}

	content := v.ReadFile("note/example.md")
	if !strings.Contains(content, "### Log\n- log entry") {
		t.Fatalf("heading and entry not created:\n%s", content)
	}

	// A second add now targets the existing heading without creating another.
	again := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"text":           "- second entry",
			"to":             "note/example",
			"heading":        "### Log",
			"create-heading": true,
		},
	})
	if !again.OK {
		t.Fatalf("second HandleAdd failed: %#v", again.Error)
	}
	content = v.ReadFile("note/example.md")
	if got := strings.Count(content, "### Log"); got != 1 {
		t.Fatalf("expected exactly one Log heading, got %d:\n%s", got, content)
	}
	if !strings.Contains(content, "- second entry") {
		t.Fatalf("second entry missing:\n%s", content)
	}
}

func TestHandleAddCreateHeadingRejectsSlugSpecs(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks
`)
	reindexForEditTest(t, v.Path)

	result := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"text":           "- entry",
			"to":             "note/example",
			"heading":        "note/example#missing",
			"create-heading": true,
		},
	})
	if result.OK {
		t.Fatalf("expected error for section-ID heading spec, got success: %#v", result.Data)
	}

	noHeading := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"text":           "- entry",
			"to":             "note/example",
			"create-heading": true,
		},
	})
	if noHeading.OK || noHeading.Error == nil || noHeading.Error.Code != "INVALID_INPUT" {
		t.Fatalf("expected INVALID_INPUT for create-heading without heading, got: %#v", noHeading.Error)
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
