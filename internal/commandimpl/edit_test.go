package commandimpl

import (
	"context"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleEditScopesMatchesToSectionSubtree(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

# Target
unique phrase
## Child
child only
# Other
unique phrase
`)
	reindexForEditTest(t, v.Path)

	result := HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args: map[string]any{
			"path":    "note/example#target",
			"old_str": "unique phrase",
			"new_str": "changed phrase",
		},
	})
	if !result.OK {
		t.Fatalf("HandleEdit() failed: %#v", result.Error)
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		t.Fatalf("Data = %#v, want map", result.Data)
	}
	if got, want := data["line"], 7; got != want {
		t.Fatalf("line = %#v, want %d", got, want)
	}

	content := v.ReadFile("note/example.md")
	if !strings.Contains(content, "# Target\nchanged phrase\n") {
		t.Fatalf("target section was not updated:\n%s", content)
	}
	if !strings.Contains(content, "# Other\nunique phrase\n") {
		t.Fatalf("outside section should remain unchanged:\n%s", content)
	}
}

func TestHandleEditSectionScopeKeepsAmbiguityWithinSubtree(t *testing.T) {
	t.Parallel()

	v := newSectionEditVault(t, `---
type: note
title: Example
---

# Target
repeat me
## Child
repeat me
# Other
repeat me
`)
	reindexForEditTest(t, v.Path)

	result := HandleEdit(context.Background(), commandexec.Request{
		VaultPath: v.Path,
		Confirm:   true,
		Args: map[string]any{
			"path":    "note/example#target",
			"old_str": "repeat me",
			"new_str": "changed",
		},
	})
	if result.OK {
		t.Fatalf("HandleEdit() succeeded, want scoped ambiguity failure: %#v", result.Data)
	}
	if result.Error == nil {
		t.Fatalf("HandleEdit() error missing: %#v", result)
	}
	if result.Error.Code != codes.ErrMultipleMatches {
		t.Fatalf("error code = %q, want %q", result.Error.Code, codes.ErrMultipleMatches)
	}
	if !strings.Contains(result.Error.Message, "2 times") {
		t.Fatalf("error message = %q, want scoped count", result.Error.Message)
	}
}

func newSectionEditVault(t *testing.T, noteContent string) *testutil.TestVault {
	t.Helper()

	return testutil.NewTestVault(t).
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
		WithFile("note/example.md", noteContent).
		Build()
}

func reindexForEditTest(t *testing.T, vaultPath string) {
	t.Helper()

	result := HandleReindex(context.Background(), commandexec.Request{
		VaultPath: vaultPath,
		Args: map[string]any{
			"full": true,
		},
	})
	if !result.OK {
		t.Fatalf("HandleReindex() failed: %#v", result.Error)
	}
}
