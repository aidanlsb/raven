package app

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/testutil"
)

// runInvoked executes a command through the shared invoker so validation,
// preview/apply normalization, and the mutation-phase annotator all run, exactly
// as they do for CLI and MCP callers.
func runInvoked(t *testing.T, vaultPath, commandID string, args map[string]any, mutate func(*commandexec.Request)) commandexec.Result {
	t.Helper()
	req := commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
	}
	if mutate != nil {
		mutate(&req)
	}
	return CommandInvoker().Execute(context.Background(), req)
}

func requirePhase(t *testing.T, result commandexec.Result, want commandexec.MutationPhase) {
	t.Helper()
	if !result.OK {
		t.Fatalf("command failed: %#v", result.Error)
	}
	if result.Meta == nil || result.Meta.Mutation == nil {
		t.Fatalf("meta.mutation missing; want phase=%q (data=%#v)", want, result.Data)
	}
	if got := result.Meta.Mutation.Phase; got != want {
		t.Fatalf("mutation phase = %q, want %q", got, want)
	}
}

func requireNoPhase(t *testing.T, result commandexec.Result) {
	t.Helper()
	if !result.OK {
		t.Fatalf("command failed: %#v", result.Error)
	}
	if result.Meta != nil && result.Meta.Mutation != nil {
		t.Fatalf("meta.mutation = %#v, want none for read-only command", result.Meta.Mutation)
	}
}

func buildPhaseVault(t *testing.T) *testutil.TestVault {
	t.Helper()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nBody line one.\n\n- task @priority(low)\n").
		WithFile("projects/scratch.md", "---\ntype: project\ntitle: Scratch\n---\n").
		Build()
	reindexPhaseVault(t, v.Path)
	return v
}

func reindexPhaseVault(t *testing.T, vaultPath string) {
	t.Helper()
	res := CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "reindex",
		VaultPath: vaultPath,
		Args:      map[string]any{"full": true},
	})
	if !res.OK {
		t.Fatalf("reindex failed: %#v", res.Error)
	}
}

func withConfirm(req *commandexec.Request) { req.Args["confirm"] = true }
func withDryRun(req *commandexec.Request)  { req.Args["dry-run"] = true }

func TestMutationPhaseSingleObjectWrites(t *testing.T) {
	t.Parallel()

	t.Run("edit applied", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "edit", map[string]any{
			"path": "projects/roadmap", "old_str": "Body line one.", "new_str": "Body line two.",
		}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})

	t.Run("edit dry-run previews", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "edit", map[string]any{
			"path": "projects/roadmap", "old_str": "Body line one.", "new_str": "Body line two.",
		}, withDryRun)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("set applied", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "set", map[string]any{
			"object_id": "projects/roadmap", "fields": map[string]any{"status": "paused"},
		}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})

	t.Run("set dry-run previews", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "set", map[string]any{
			"object_id": "projects/roadmap", "fields": map[string]any{"status": "paused"},
		}, withDryRun)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("delete applies immediately for JSON/agent callers", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "delete", map[string]any{"object_id": "projects/scratch"}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})

	t.Run("delete dry-run previews", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "delete", map[string]any{"object_id": "projects/scratch"}, withDryRun)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("new applied", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "new", map[string]any{"type": "person", "title": "Thor"}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})

	t.Run("add applied", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "add", map[string]any{"text": "- captured", "to": "projects/roadmap"}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseBulkWrites(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name      string
		commandID string
		args      map[string]any
	}{
		{"delete", "delete", map[string]any{"stdin": true, "object_ids": []any{"projects/scratch"}}},
		{"set", "set", map[string]any{"stdin": true, "object_ids": []any{"projects/roadmap"}, "fields": map[string]any{"status": "done"}}},
		{"add", "add", map[string]any{"stdin": true, "object_ids": []any{"projects/roadmap"}, "text": "- bulk line"}},
		{"move", "move", map[string]any{"stdin": true, "object_ids": []any{"projects/scratch"}, "destination": "archive/"}},
	}

	for _, tc := range cases {
		t.Run(tc.name+" preview by default", func(t *testing.T) {
			t.Parallel()
			v := buildPhaseVault(t)
			res := runInvoked(t, v.Path, tc.commandID, cloneArgs(tc.args), nil)
			requirePhase(t, res, commandexec.MutationPhasePreview)
		})

		t.Run(tc.name+" applies with confirm", func(t *testing.T) {
			t.Parallel()
			v := buildPhaseVault(t)
			res := runInvoked(t, v.Path, tc.commandID, cloneArgs(tc.args), withConfirm)
			requirePhase(t, res, commandexec.MutationPhaseApplied)
		})
	}
}

func TestMutationPhaseMoveNeedsConfirmPreviews(t *testing.T) {
	t.Parallel()
	v := buildPhaseVault(t)

	// Moving a person file into the project type's default directory is blocked
	// pending confirmation; nothing is written, so the phase must be preview.
	res := runInvoked(t, v.Path, "move", map[string]any{
		"source": "people/freya", "destination": "projects/freya",
	}, nil)
	requirePhase(t, res, commandexec.MutationPhasePreview)
	data, _ := res.Data.(map[string]interface{})
	if needs, _ := data["needs_confirm"].(bool); !needs {
		t.Fatalf("expected needs_confirm=true, got data=%#v", data)
	}
}

func TestMutationPhaseReclassify(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) *testutil.TestVault {
		t.Helper()
		v := testutil.NewTestVault(t).
			WithSchema(`version: 1
types:
  alpha:
    default_path: alpha/
    name_field: title
    fields:
      title:
        type: string
        required: true
      note:
        type: string
  beta:
    default_path: beta/
    name_field: title
    fields:
      title:
        type: string
        required: true
`).
			WithFile("alpha/one.md", "---\ntype: alpha\ntitle: One\nnote: keepsake\n---\n").
			Build()
		reindexPhaseVault(t, v.Path)
		return v
	}

	t.Run("needs confirm previews", func(t *testing.T) {
		t.Parallel()
		v := build(t)
		// Dropping the 'note' field requires --force; without it nothing is written.
		res := runInvoked(t, v.Path, "reclassify", map[string]any{"object": "alpha/one", "new-type": "beta"}, nil)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("applies with force", func(t *testing.T) {
		t.Parallel()
		v := build(t)
		res := runInvoked(t, v.Path, "reclassify", map[string]any{"object": "alpha/one", "new-type": "beta", "force": true}, nil)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseUpdateTrait(t *testing.T) {
	t.Parallel()
	v := buildPhaseVault(t)

	ids := runInvoked(t, v.Path, "query", map[string]any{"query_string": "trait:priority", "ids": true}, nil)
	if !ids.OK {
		t.Fatalf("query ids failed: %#v", ids.Error)
	}
	payload, ok := ids.Data.(commandpayload.QueryIDsResult)
	if !ok {
		t.Fatalf("expected QueryIDsResult, got %#v", ids.Data)
	}
	if len(payload.IDs) == 0 {
		t.Fatalf("expected a priority trait id, got %#v", payload.IDs)
	}
	traitID := payload.IDs[0]

	preview := runInvoked(t, v.Path, "update", map[string]any{"trait_id": traitID, "value": "high"}, withDryRun)
	requirePhase(t, preview, commandexec.MutationPhasePreview)

	applied := runInvoked(t, v.Path, "update", map[string]any{"trait_id": traitID, "value": "high"}, nil)
	requirePhase(t, applied, commandexec.MutationPhaseApplied)
}

func TestMutationPhaseImport(t *testing.T) {
	t.Parallel()

	t.Run("dry-run previews", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "import", map[string]any{"type": "person", "dry-run": true}, func(req *commandexec.Request) {
			req.Stdin = []byte(`[{"name":"Imported One"}]`)
		})
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("applies without dry-run", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "import", map[string]any{"type": "person"}, func(req *commandexec.Request) {
			req.Stdin = []byte(`[{"name":"Imported Two"}]`)
		})
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseCheckFix(t *testing.T) {
	t.Parallel()

	build := func(t *testing.T) *testutil.TestVault {
		t.Helper()
		v := testutil.NewTestVault(t).
			WithSchema(testutil.PersonProjectSchema()).
			WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
			WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nowner: \"[[freya]]\"\n---\n").
			Build()
		reindexPhaseVault(t, v.Path)
		return v
	}

	t.Run("preview by default", func(t *testing.T) {
		t.Parallel()
		v := build(t)
		res := runInvoked(t, v.Path, "check_fix", map[string]any{}, nil)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("applies with confirm", func(t *testing.T) {
		t.Parallel()
		v := build(t)
		res := runInvoked(t, v.Path, "check_fix", map[string]any{}, withConfirm)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseSchemaRenameField(t *testing.T) {
	t.Parallel()

	rename := func() map[string]any {
		return map[string]any{"type_name": "project", "old_field": "status", "new_field": "state"}
	}

	t.Run("preview by default", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "schema_rename_field", rename(), nil)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("applies with confirm", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "schema_rename_field", rename(), withConfirm)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseQueryApplyDelegates(t *testing.T) {
	t.Parallel()

	t.Run("preview by default", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "query", map[string]any{
			"query_string": "type:project", "apply": []string{"set status=done"},
		}, nil)
		requirePhase(t, res, commandexec.MutationPhasePreview)
	})

	t.Run("applies with confirm", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		res := runInvoked(t, v.Path, "query", map[string]any{
			"query_string": "type:project", "apply": []string{"set status=done"},
		}, withConfirm)
		requirePhase(t, res, commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseQueryApplyNoMatches(t *testing.T) {
	t.Parallel()

	// No project has status==done, so --apply resolves to zero targets and the
	// handler reports the phase directly (no nested command runs).
	args := func() map[string]any {
		return map[string]any{"query_string": "type:project .status==done", "apply": []string{"set status=paused"}}
	}

	t.Run("preview by default", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		requirePhase(t, runInvoked(t, v.Path, "query", args(), nil), commandexec.MutationPhasePreview)
	})

	t.Run("applied with confirm", func(t *testing.T) {
		t.Parallel()
		v := buildPhaseVault(t)
		requirePhase(t, runInvoked(t, v.Path, "query", args(), withConfirm), commandexec.MutationPhaseApplied)
	})
}

func TestMutationPhaseAbsentForReads(t *testing.T) {
	t.Parallel()
	v := buildPhaseVault(t)

	// Plain query is read-only even though it shares the preview-default policy
	// with query --apply.
	requireNoPhase(t, runInvoked(t, v.Path, "query", map[string]any{"query_string": "type:project"}, nil))
	// Discovery/read commands never carry a mutation phase.
	requireNoPhase(t, runInvoked(t, v.Path, "schema", map[string]any{}, nil))
	requireNoPhase(t, runInvoked(t, v.Path, "read", map[string]any{"path": "projects/roadmap"}, nil))
}

func cloneArgs(args map[string]any) map[string]any {
	out := make(map[string]any, len(args))
	for k, v := range args {
		out[k] = v
	}
	return out
}
