package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleEditUsesNormalizedConfirmField(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

old body
`)
	reindexForEditTest(t, v.Path)

	result := HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args: map[string]any{
			"path":    "note/example",
			"old_str": "old body",
			"new_str": "new body",
			"confirm": true,
		},
	})
	if !result.OK {
		t.Fatalf("HandleEdit() failed: %#v", result.Error)
	}
	if got := dataString(result, "status"); got != "preview" {
		t.Fatalf("status = %q, want preview", got)
	}
	if strings.Contains(v.ReadFile("note/example.md"), "new body") {
		t.Fatal("edit applied even though req.Confirm was false")
	}
}

func TestHandleAddBulkUsesNormalizedConfirmField(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

body
`)
	reindexForEditTest(t, v.Path)

	args := map[string]any{
		"text":       "bulk note",
		"stdin":      true,
		"object_ids": []interface{}{"note/example"},
	}
	preview := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      args,
	})
	if !preview.OK {
		t.Fatalf("HandleAdd preview failed: %#v", preview.Error)
	}
	if !dataBool(preview, "preview") {
		t.Fatalf("preview data = %#v, want preview=true", preview.Data)
	}
	if strings.Contains(v.ReadFile("note/example.md"), "bulk note") {
		t.Fatal("bulk add applied during preview")
	}

	apply := HandleAdd(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args:      args,
	})
	if !apply.OK {
		t.Fatalf("HandleAdd apply failed: %#v", apply.Error)
	}
	if !strings.Contains(v.ReadFile("note/example.md"), "bulk note") {
		t.Fatal("bulk add did not apply when req.Confirm was true")
	}
}

func TestHandleCheckFixUsesNormalizedConfirmField(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", `---
type: person
name: Freya
---`).
		WithFile("projects/roadmap.md", `---
type: project
title: Roadmap
owner: "[[freya]]"
---`).
		Build()
	reindexForEditTest(t, v.Path)

	args := map[string]any{"fix": true}
	preview := HandleCheck(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      args,
	})
	if !preview.OK {
		t.Fatalf("HandleCheck preview failed: %#v", preview.Error)
	}
	if !dataBool(preview, "preview") {
		t.Fatalf("preview data = %#v, want preview=true", preview.Data)
	}
	if strings.Contains(v.ReadFile("projects/roadmap.md"), "[[people/freya]]") {
		t.Fatal("check fix applied during preview")
	}

	apply := HandleCheck(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args:      args,
	})
	if !apply.OK {
		t.Fatalf("HandleCheck apply failed: %#v", apply.Error)
	}
	if dataBool(apply, "preview") {
		t.Fatalf("apply data = %#v, want preview=false", apply.Data)
	}
	v.AssertFileContains("projects/roadmap.md", `owner: "[[people/freya]]"`)
}

func dataBool(result commandexec.Result, key string) bool {
	data, _ := result.Data.(map[string]interface{})
	value, _ := data[key].(bool)
	return value
}

func dataString(result commandexec.Result, key string) string {
	data, _ := result.Data.(map[string]interface{})
	value, _ := data[key].(string)
	return value
}
