package commandimpl

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/testutil"
)

// newQueryPayloadVault builds a vault exercising every query result mode:
// a typed object with a section, a trait on that section, and a referenced
// asset.
func newQueryPayloadVault(t *testing.T) *testutil.TestVault {
	t.Helper()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/raven.md", `---
type: project
title: Raven
status: active
---
# Raven

## Tasks

Ship it @priority(high). See [[assets/pdfs/paper.pdf]].
Read [the spec](docs/spec.pdf).
`).
		WithFile("assets/pdfs/paper.pdf", "%PDF-1.7\nhello").
		Build()

	reindexForEditTest(t, v.Path)
	return v
}

func runQueryHandler(t *testing.T, vaultPath string, args map[string]any) commandexec.Result {
	t.Helper()
	result := HandleQuery(context.Background(), commandexec.Request{
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
	})
	if !result.OK {
		t.Fatalf("HandleQuery(%v) failed: %#v", args, result.Error)
	}
	return result
}

// marshalToMap serializes the typed payload the way the JSON envelope does and
// decodes it back into a generic map so tests can assert the stable wire shape.
func marshalToMap(t *testing.T, data interface{}) map[string]interface{} {
	t.Helper()
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	return out
}

func assertKeys(t *testing.T, got map[string]interface{}, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("key count = %d %v, want %d %v", len(got), keysOf(got), len(want), want)
	}
	for _, key := range want {
		if _, ok := got[key]; !ok {
			t.Fatalf("missing key %q in %v", key, keysOf(got))
		}
	}
}

func keysOf(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	return keys
}

func TestHandleQueryTypedObjectResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "type:project"})

	payload, ok := result.Data.(commandpayload.QueryObjectResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryObjectResult", result.Data)
	}
	if payload.QueryKind != "type" || payload.Type != "project" {
		t.Fatalf("query_kind/type = %q/%q, want type/project", payload.QueryKind, payload.Type)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "projects/raven" {
		t.Fatalf("items = %#v, want one projects/raven item", payload.Items)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query_kind", "type", "items", "total", "returned", "offset", "limit", "has_more"})
	item := wire["items"].([]interface{})[0].(map[string]interface{})
	assertKeys(t, item, []string{"num", "id", "type", "fields", "file_path", "line"})
}

func TestHandleQueryTypedTraitResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "trait:priority"})

	payload, ok := result.Data.(commandpayload.QueryTraitResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryTraitResult", result.Data)
	}
	if payload.QueryKind != "trait" || payload.Trait != "priority" {
		t.Fatalf("query_kind/trait = %q/%q, want trait/priority", payload.QueryKind, payload.Trait)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %#v, want one trait item", payload.Items)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query_kind", "trait", "items", "total", "returned", "offset", "limit", "has_more"})
	item := wire["items"].([]interface{})[0].(map[string]interface{})
	assertKeys(t, item, []string{"num", "id", "trait_type", "value", "content", "file_path", "line", "object_id"})
	if item["value"] != "high" {
		t.Fatalf("trait value = %#v, want high", item["value"])
	}
}

func TestHandleQueryTypedAssetResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "asset .extension==pdf"})

	payload, ok := result.Data.(commandpayload.QueryAssetResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryAssetResult", result.Data)
	}
	if payload.QueryKind != "asset" {
		t.Fatalf("query_kind = %q, want asset", payload.QueryKind)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "assets/pdfs/paper.pdf" {
		t.Fatalf("items = %#v, want one paper.pdf item", payload.Items)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query_kind", "items", "total", "returned", "offset", "limit", "has_more"})
	item := wire["items"].([]interface{})[0].(map[string]interface{})
	assertKeys(t, item, []string{"num", "id", "file_path", "filename", "extension", "media_type", "size_bytes"})
}

func TestHandleQueryTypedSectionResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "section .title==Tasks"})

	payload, ok := result.Data.(commandpayload.QuerySectionResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QuerySectionResult", result.Data)
	}
	if payload.QueryKind != "section" {
		t.Fatalf("query_kind = %q, want section", payload.QueryKind)
	}
	if len(payload.Items) != 1 {
		t.Fatalf("items = %#v, want one Tasks section", payload.Items)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query_kind", "items", "total", "returned", "offset", "limit", "has_more"})
	item := wire["items"].([]interface{})[0].(map[string]interface{})
	assertKeys(t, item, []string{
		"num", "id", "file_object_id", "file_path", "slug", "title", "level",
		"line_start", "line_end", "direct_line_end", "subtree_line_end", "parent_section_id",
	})
}

func TestHandleQueryTypedLinkResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "link .ext==pdf"})

	payload, ok := result.Data.(commandpayload.QueryLinkResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryLinkResult", result.Data)
	}
	if payload.QueryKind != "link" {
		t.Fatalf("query_kind = %q, want link", payload.QueryKind)
	}
	if len(payload.Items) != 1 || payload.Items[0].RawTarget != "docs/spec.pdf" {
		t.Fatalf("items = %#v, want one docs/spec.pdf edge", payload.Items)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query_kind", "items", "total", "returned", "offset", "limit", "has_more"})
	item := wire["items"].([]interface{})[0].(map[string]interface{})
	assertKeys(t, item, []string{
		"num", "source_id", "source_type", "file_path", "line", "position_start",
		"position_end", "raw_target", "display", "is_image", "scheme", "ext", "normalized_key",
	})
}

func TestHandleQueryTypedIDsResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	result := runQueryHandler(t, v.Path, map[string]any{"query_string": "type:project", "ids": true})

	payload, ok := result.Data.(commandpayload.QueryIDsResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.QueryIDsResult", result.Data)
	}
	if len(payload.IDs) != 1 || payload.IDs[0] != "projects/raven" {
		t.Fatalf("ids = %#v, want [projects/raven]", payload.IDs)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"ids", "total", "returned", "offset", "limit", "has_more"})
}

func TestHandleQueryTypedCountResult(t *testing.T) {
	t.Parallel()
	v := newQueryPayloadVault(t)

	t.Run("type", func(t *testing.T) {
		result := runQueryHandler(t, v.Path, map[string]any{"query_string": "type:project", "count-only": true})
		payload, ok := result.Data.(commandpayload.QueryCountResult)
		if !ok {
			t.Fatalf("Data type = %T, want commandpayload.QueryCountResult", result.Data)
		}
		if payload.QueryKind != "type" || payload.Type != "project" || payload.Total != 1 {
			t.Fatalf("count payload = %#v, want type/project total=1", payload)
		}
		assertKeys(t, marshalToMap(t, payload), []string{"query_kind", "type", "total"})
	})

	t.Run("trait", func(t *testing.T) {
		result := runQueryHandler(t, v.Path, map[string]any{"query_string": "trait:priority", "count-only": true})
		payload := result.Data.(commandpayload.QueryCountResult)
		if payload.QueryKind != "trait" || payload.Trait != "priority" {
			t.Fatalf("count payload = %#v, want trait/priority", payload)
		}
		assertKeys(t, marshalToMap(t, payload), []string{"query_kind", "trait", "total"})
	})

	t.Run("asset", func(t *testing.T) {
		result := runQueryHandler(t, v.Path, map[string]any{"query_string": "asset", "count-only": true})
		payload := result.Data.(commandpayload.QueryCountResult)
		if payload.QueryKind != "asset" {
			t.Fatalf("count payload = %#v, want asset", payload)
		}
		assertKeys(t, marshalToMap(t, payload), []string{"query_kind", "total"})
	})

	t.Run("link", func(t *testing.T) {
		result := runQueryHandler(t, v.Path, map[string]any{"query_string": "link", "count-only": true})
		payload := result.Data.(commandpayload.QueryCountResult)
		if payload.QueryKind != "link" || payload.Total != 1 {
			t.Fatalf("count payload = %#v, want link total=1", payload)
		}
		assertKeys(t, marshalToMap(t, payload), []string{"query_kind", "total"})
	})
}
