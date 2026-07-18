package commandimpl

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/testutil"
)

// newReadPayloadVault builds a vault where a project references a person and is
// referenced back, and carries a heading-derived section, so read and search
// exercise references, backlinks, and section outlines.
func newReadPayloadVault(t *testing.T) *testutil.TestVault {
	t.Helper()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/raven.md", `---
type: project
title: Raven
status: active
owner: people/alice
---
# Raven

## Tasks

Ship the roadmap. See [[people/alice]].
`).
		WithFile("people/alice.md", `---
type: person
name: Alice
---
# Alice

Leads [[projects/raven]].
`).
		Build()

	reindexForEditTest(t, v.Path)
	return v
}

func runReadHandler(t *testing.T, vaultPath string, args map[string]any) commandexec.Result {
	t.Helper()
	result := HandleRead(context.Background(), commandexec.Request{
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
	})
	if !result.OK {
		t.Fatalf("HandleRead(%v) failed: %#v", args, result.Error)
	}
	return result
}

func TestHandleSearchTypedResult(t *testing.T) {
	t.Parallel()
	v := newReadPayloadVault(t)

	result := HandleSearch(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Caller:    commandexec.CallerCLI,
		Args:      map[string]any{"query": "roadmap"},
	})
	if !result.OK {
		t.Fatalf("HandleSearch failed: %#v", result.Error)
	}

	payload, ok := result.Data.(commandpayload.SearchResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.SearchResult", result.Data)
	}
	if payload.Query != "roadmap" {
		t.Fatalf("query = %q, want roadmap", payload.Query)
	}
	if len(payload.Results) == 0 {
		t.Fatalf("results = %#v, want at least one match", payload.Results)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"query", "results"})
	item := wire["results"].([]interface{})[0].(map[string]interface{})
	// Object matches carry only the core columns; section-only fields are absent.
	assertKeys(t, item, []string{"object_id", "title", "file_path", "snippet", "rank"})
}

func TestHandleReadTypedContentResult(t *testing.T) {
	t.Parallel()
	v := newReadPayloadVault(t)

	result := runReadHandler(t, v.Path, map[string]any{"path": "projects/raven"})

	payload, ok := result.Data.(commandpayload.ReadContentResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.ReadContentResult", result.Data)
	}
	if payload.ObjectID != "projects/raven" {
		t.Fatalf("object_id = %q, want projects/raven", payload.ObjectID)
	}
	if len(payload.References) == 0 {
		t.Fatalf("references = %#v, want at least one outgoing reference", payload.References)
	}
	if len(payload.Backlinks) == 0 {
		t.Fatalf("backlinks = %#v, want at least one backlink", payload.Backlinks)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"object_id", "path", "content", "line_count", "references", "backlinks"})
}

func TestHandleReadTypedRawResult(t *testing.T) {
	t.Parallel()
	v := newReadPayloadVault(t)

	result := runReadHandler(t, v.Path, map[string]any{
		"path":       "projects/raven",
		"raw":        true,
		"lines":      true,
		"start-line": 1,
		"end-line":   5,
	})

	payload, ok := result.Data.(commandpayload.ReadRawResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.ReadRawResult", result.Data)
	}
	if payload.StartLine != 1 || payload.EndLine != 5 {
		t.Fatalf("range = %d-%d, want 1-5", payload.StartLine, payload.EndLine)
	}
	if len(payload.Lines) == 0 {
		t.Fatalf("lines = %#v, want structured lines", payload.Lines)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"object_id", "path", "content", "line_count", "start_line", "end_line", "lines"})
}

func TestHandleReadTypedRawResultFullFile(t *testing.T) {
	t.Parallel()
	v := newReadPayloadVault(t)

	result := runReadHandler(t, v.Path, map[string]any{
		"path": "projects/raven",
		"raw":  true,
	})

	payload, ok := result.Data.(commandpayload.ReadRawResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.ReadRawResult", result.Data)
	}

	// A full-file raw read omits the range and structured-line fields.
	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"object_id", "path", "content", "line_count"})
}

func TestHandleReadTypedSectionsResult(t *testing.T) {
	t.Parallel()
	v := newReadPayloadVault(t)

	result := runReadHandler(t, v.Path, map[string]any{
		"path":     "projects/raven",
		"sections": true,
	})

	payload, ok := result.Data.(commandpayload.ReadSectionsResult)
	if !ok {
		t.Fatalf("Data type = %T, want commandpayload.ReadSectionsResult", result.Data)
	}
	if len(payload.Sections) == 0 {
		t.Fatalf("sections = %#v, want at least one section", payload.Sections)
	}

	wire := marshalToMap(t, payload)
	assertKeys(t, wire, []string{"object_id", "path", "sections"})
	item := wire["sections"].([]interface{})[0].(map[string]interface{})
	// Line bounds and parent pointer are omitted when unset, so assert the
	// always-present keys individually rather than an exact key set.
	for _, key := range []string{"id", "slug", "title", "level", "line_start"} {
		if _, ok := item[key]; !ok {
			t.Fatalf("missing section key %q in %v", key, keysOf(item))
		}
	}
}
