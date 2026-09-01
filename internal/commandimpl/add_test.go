package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
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
	previewData, _ := preview.Data.(commandpayload.AddBulkPreviewResult)
	items := previewData.Items
	if len(items) != 1 || items[0].ID != "note/example#tasks" {
		t.Fatalf("preview items = %#v, want one item for note/example#tasks", items)
	}
	if len(previewData.Warnings) > 0 {
		t.Fatalf("preview warnings = %#v, want none", previewData.Warnings)
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
	data, _ := apply.Data.(commandpayload.AddBulkResult)
	if data.Added != 2 {
		t.Fatalf("added = %d, want 2 (data = %#v)", data.Added, data)
	}
	if got := strings.Count(v.ReadFile("note/example.md"), "- mixed line"); got != 2 {
		t.Fatalf("expected 2 appended lines, got %d:\n%s", got, v.ReadFile("note/example.md"))
	}
}

func TestHandleAddRejectsRemovedHeadingArguments(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

## Tasks

- existing task
`)
	reindexForEditTest(t, v.Path)

	tests := []struct {
		name string
		args map[string]any
	}{
		{
			name: "heading",
			args: map[string]any{
				"text":    "- log entry",
				"to":      "note/example",
				"heading": "### Log",
			},
		},
		{
			name: "create heading",
			args: map[string]any{
				"text":           "- log entry",
				"to":             "note/example",
				"create-heading": true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HandleAdd(context.Background(), commandexec.Request{
				VaultPath: v.Path,
				Args:      tt.args,
			})
			if result.OK || result.Error == nil || result.Error.Code != "INVALID_INPUT" {
				t.Fatalf("expected INVALID_INPUT, got: %#v", result)
			}
			if !strings.Contains(result.Error.Suggestion, "rvn section create") {
				t.Fatalf("suggestion = %q, want section create guidance", result.Error.Suggestion)
			}
		})
	}
}

func TestHandleAddRejectsHeadingContent(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

Body
`)
	reindexForEditTest(t, v.Path)

	for _, text := range []string{"## Log", "Title\n---", "Body line\n\n### Nested"} {
		result := HandleAdd(context.Background(), commandexec.Request{
			VaultPath: v.Path,
			Args: map[string]any{
				"text": text,
				"to":   "note/example",
			},
		})
		if result.OK || result.Error == nil || result.Error.Code != "INVALID_INPUT" {
			t.Fatalf("text %q: expected INVALID_INPUT, got %#v", text, result)
		}
		if !strings.Contains(result.Error.Suggestion, "rvn section create") {
			t.Fatalf("text %q: suggestion = %q", text, result.Error.Suggestion)
		}
	}
	if got := v.ReadFile("note/example.md"); strings.Contains(got, "## Log") || strings.Contains(got, "### Nested") {
		t.Fatalf("rejected heading content changed file:\n%s", got)
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
			"references": []interface{}{"note/example", "note/example#tasks"},
			"field": map[string]interface{}{
				"title": "Renamed",
			},
		},
	})
	if !result.OK {
		t.Fatalf("HandleSet preview failed: %#v", result.Error)
	}
	data, _ := result.Data.(commandpayload.SetBulkPreviewResult)
	warnings := data.Warnings
	if len(warnings) != 1 || warnings[0].Code != warnSectionSkipped {
		t.Fatalf("warnings = %#v, want one SECTION_SKIPPED warning", warnings)
	}
	items := data.Items
	if len(items) != 1 || items[0].ID != "note/example" {
		t.Fatalf("preview items = %#v, want only note/example", items)
	}
}
