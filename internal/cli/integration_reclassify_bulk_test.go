//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_ReclassifyBulkPreviewAndApply(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  note:
    default_path: notes/
    fields:
      title:
        type: string
      legacy:
        type: string
  doc:
    default_path: docs/
    fields:
      title:
        type: string
        required: true
`).
		WithFile("notes/ready.md", `---
type: note
title: Ready
legacy: remove me
---

Ready body.
`).
		WithFile("notes/missing.md", `---
type: note
---

Missing title.
`).
		WithFile("refs/link.md", `---
type: note
title: Link
---

See [[notes/ready]].
`).
		Build()
	v.RunCLI("reindex", "--full").MustSucceed(t)

	stdin := "notes/ready\nnotes/missing\n"
	preview := v.RunCLIWithStdin(stdin, "reclassify", "doc", "--stdin")
	preview.MustSucceed(t)
	if got := preview.Data["preview"]; got != true {
		t.Fatalf("preview flag = %#v, want true; response=%s", got, preview.RawJSON)
	}
	previewItems := reclassifyBulkItems(t, preview, "items")
	if len(previewItems) != 1 {
		t.Fatalf("preview items len = %d, want 1; response=%s", len(previewItems), preview.RawJSON)
	}
	readyPreview := previewItems[0]
	if readyPreview["new_path"] != "docs/ready.md" || readyPreview["moved"] != true {
		t.Fatalf("ready preview move = %#v, want docs/ready.md", readyPreview)
	}
	if readyPreview["needs_confirm"] != true {
		t.Fatalf("ready preview should require --force for dropped fields: %#v", readyPreview)
	}
	dropped, ok := readyPreview["dropped_fields"].([]interface{})
	if !ok || len(dropped) != 1 || dropped[0] != "legacy" {
		t.Fatalf("dropped_fields = %#v, want [legacy]", readyPreview["dropped_fields"])
	}
	updatedRefs, ok := readyPreview["updated_refs"].([]interface{})
	if !ok || len(updatedRefs) != 1 || updatedRefs[0] != "refs/link" {
		t.Fatalf("updated_refs = %#v, want [refs/link]", readyPreview["updated_refs"])
	}
	previewFailures := reclassifyBulkItems(t, preview, "skipped")
	if len(previewFailures) != 1 || previewFailures[0]["error_code"] != "REQUIRED_FIELD_MISSING" {
		t.Fatalf("preview failures = %#v, want required-field failure", previewFailures)
	}
	v.AssertFileContains("notes/ready.md", "type: note")
	v.AssertFileNotExists("docs/ready.md")
	v.AssertFileContains("refs/link.md", "[[notes/ready]]")

	blocked := v.RunCLIWithStdin(stdin, "reclassify", "doc", "--stdin", "--confirm")
	blocked.MustSucceed(t)
	if blocked.Data["reclassified"] != float64(0) || blocked.Data["skipped"] != float64(1) || blocked.Data["errors"] != float64(1) {
		t.Fatalf("blocked summary = %#v, want reclassified=0 skipped=1 errors=1", blocked.Data)
	}
	v.AssertFileContains("notes/ready.md", "type: note")

	applied := v.RunCLIWithStdin(stdin, "reclassify", "doc", "--stdin", "--confirm", "--force")
	applied.MustSucceed(t)
	if applied.Data["reclassified"] != float64(1) || applied.Data["skipped"] != float64(0) || applied.Data["errors"] != float64(1) {
		t.Fatalf("applied summary = %#v, want reclassified=1 skipped=0 errors=1", applied.Data)
	}
	appliedItems := reclassifyBulkItems(t, applied, "items")
	if len(appliedItems) != 2 {
		t.Fatalf("applied items len = %d, want 2; response=%s", len(appliedItems), applied.RawJSON)
	}
	v.AssertFileNotExists("notes/ready.md")
	v.AssertFileContains("docs/ready.md", "type: doc")
	v.AssertFileNotContains("docs/ready.md", "legacy:")
	v.AssertFileContains("refs/link.md", "[[docs/ready]]")
	v.AssertFileContains("notes/missing.md", "type: note")
}

func reclassifyBulkItems(t *testing.T, result *testutil.CLIResult, key string) []map[string]interface{} {
	t.Helper()
	raw, ok := result.Data[key].([]interface{})
	if !ok {
		t.Fatalf("data.%s = %#v, want array; response=%s", key, result.Data[key], result.RawJSON)
	}
	items := make([]map[string]interface{}, 0, len(raw))
	for _, value := range raw {
		item, ok := value.(map[string]interface{})
		if !ok {
			t.Fatalf("data.%s item = %#v, want object", key, value)
		}
		items = append(items, item)
	}
	return items
}
