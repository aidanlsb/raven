package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/testutil"
)

// corruptSchemaYAML is valid enough to exist on disk but fails to parse, so
// schema.Load returns an error (as opposed to a missing schema.yaml, which
// yields a default schema with no error).
const corruptSchemaYAML = "version: 1\ntypes: [unterminated\n"

// newSchemaPolicyVault builds a vault with a single note, reindexes it, and
// then overwrites schema.yaml with a corrupt document so that subsequent
// schema.Load calls fail.
func newSchemaPolicyVault(t *testing.T) *testutil.TestVault {
	t.Helper()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  note:
    default_path: note/
    name_field: title
    fields:
      title:
        type: string
        required: true
`).
		WithFile("note/example.md", `---
type: note
title: Example
---

unique phrase
`).
		Build()

	result := HandleReindex(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      map[string]any{"full": true},
	})
	if !result.OK {
		t.Fatalf("HandleReindex() failed: %#v", result.Error)
	}

	v.WriteFile("schema.yaml", corruptSchemaYAML)
	return v
}

func TestHandleDeleteFatalOnSchemaLoadFailure(t *testing.T) {
	t.Parallel()

	v := newSchemaPolicyVault(t)

	result := HandleDelete(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      map[string]any{"reference": "note/example"},
	})

	if result.OK {
		t.Fatalf("HandleDelete() succeeded, want fatal schema failure: %#v", result.Data)
	}
	if result.Error == nil || result.Error.Code != codes.ErrSchemaInvalid {
		t.Fatalf("error = %#v, want code %q", result.Error, codes.ErrSchemaInvalid)
	}
	if !v.FileExists("note/example.md") {
		t.Fatalf("note/example.md was deleted despite schema load failure")
	}
}

func TestHandleEditFatalOnSchemaLoadFailure(t *testing.T) {
	t.Parallel()

	v := newSchemaPolicyVault(t)

	result := HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args: map[string]any{
			"reference": "note/example",
			"old_str":   "unique phrase",
			"new_str":   "changed phrase",
		},
	})

	if result.OK {
		t.Fatalf("HandleEdit() succeeded, want fatal schema failure: %#v", result.Data)
	}
	if result.Error == nil || result.Error.Code != codes.ErrSchemaInvalid {
		t.Fatalf("error = %#v, want code %q", result.Error, codes.ErrSchemaInvalid)
	}
	if got := v.ReadFile("note/example.md"); !strings.Contains(got, "unique phrase") {
		t.Fatalf("note/example.md content was mutated despite schema load failure:\n%s", got)
	}
}

func TestHandleReadWarnsOnSchemaLoadFailure(t *testing.T) {
	t.Parallel()

	v := newSchemaPolicyVault(t)

	result := HandleRead(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Args:      map[string]any{"reference": "note/example"},
	})

	if !result.OK {
		t.Fatalf("HandleRead() failed on degraded schema, want success with warning: %#v", result.Error)
	}
	found := false
	for _, w := range result.Warnings {
		if w.Code == codes.WarnSchemaLoadFailed {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("HandleRead() warnings = %#v, want %q", result.Warnings, codes.WarnSchemaLoadFailed)
	}
}

func TestHandleMoveFatalOnSchemaLoadFailure(t *testing.T) {
	t.Parallel()

	v := newSchemaPolicyVault(t)

	result := HandleMove(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args: map[string]any{
			"source":      "note/example",
			"destination": "note/renamed",
		},
	})

	if result.OK {
		t.Fatalf("HandleMove() succeeded, want fatal schema failure: %#v", result.Data)
	}
	if result.Error == nil || result.Error.Code != codes.ErrSchemaInvalid {
		t.Fatalf("error = %#v, want code %q", result.Error, codes.ErrSchemaInvalid)
	}
	if !v.FileExists("note/example.md") {
		t.Fatalf("note/example.md was moved despite schema load failure")
	}
	if v.FileExists("note/renamed.md") {
		t.Fatalf("note/renamed.md was created despite schema load failure")
	}
}
