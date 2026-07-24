package commandimpl

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestApplyChangeSetAutoReindexDisabled(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("people/alice.md", "---\ntype: person\nname: [\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})

	changes := mutation.NewChangeSet()
	changes.AddChanged("people/alice.md")
	data, warnings := applyChangeSet(rt, changes)

	if len(data) != 0 {
		t.Fatalf("data = %#v, want no annotations", data)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none with auto-reindex disabled", warnings)
	}
}

func TestApplyChangeSetAutoReindexDisabledSkipsStaleMoveAnnotations(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("notes/ref.md", "See [[people/freya]].\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	indexPostMutationFiles(t, rt, "people/freya.md", "notes/ref.md")

	if err := os.MkdirAll(filepath.Join(v.Path, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	if err := os.Rename(
		filepath.Join(v.Path, "people", "freya.md"),
		filepath.Join(v.Path, "archive", "freya.md"),
	); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if err := os.WriteFile(filepath.Join(v.Path, "notes", "ref.md"), []byte("See [[archive/freya]].\n"), 0o644); err != nil {
		t.Fatalf("rewrite ref: %v", err)
	}

	changes := mutation.NewChangeSet()
	changes.AddMoved("people/freya.md", "archive/freya.md")
	changes.AddChanged("notes/ref.md")
	data, warnings := applyChangeSet(rt, changes)
	if len(data) != 0 {
		t.Fatalf("data = %#v, want no stale missing-ref annotations", data)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none with auto-reindex disabled", warnings)
	}
}

func TestApplyChangeSetReportsMissingRefsAcrossFiles(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		WithFile("notes/one.md", "See [[people/ghost]].\n").
		WithFile("notes/two.md", "See [[projects/missing]].\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	indexPostMutationFiles(t, rt, "people/alice.md")

	changes := mutation.NewChangeSet()
	changes.AddChanged("notes/one.md", "notes/two.md")
	data, warnings := applyChangeSet(rt, changes)

	if got := data["missing_refs"]; got != 2 {
		t.Fatalf("missing_refs = %#v, want 2", got)
	}
	if len(warnings) != 2 {
		t.Fatalf("warnings = %#v, want two missing-ref warnings", warnings)
	}
	for _, warning := range warnings {
		if warning.Code != codes.WarnRefTargetMissing {
			t.Fatalf("warning code = %q, want %q", warning.Code, codes.WarnRefTargetMissing)
		}
	}
}

func TestApplyChangeSetProjectsMoveAndDelete(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/obsolete.md", "---\ntype: project\ntitle: Obsolete\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	indexPostMutationFiles(t, rt, "people/freya.md", "projects/obsolete.md")

	if err := os.MkdirAll(filepath.Join(v.Path, "archive"), 0o755); err != nil {
		t.Fatalf("mkdir archive: %v", err)
	}
	if err := os.Rename(
		filepath.Join(v.Path, "people", "freya.md"),
		filepath.Join(v.Path, "archive", "freya.md"),
	); err != nil {
		t.Fatalf("move file: %v", err)
	}
	if err := os.Remove(filepath.Join(v.Path, "projects", "obsolete.md")); err != nil {
		t.Fatalf("delete file: %v", err)
	}

	changes := mutation.NewChangeSet()
	changes.AddMoved("people/freya.md", "archive/freya.md")
	changes.AddDeleted("projects/obsolete.md")
	data, warnings := applyChangeSet(rt, changes)
	if len(data) != 0 {
		t.Fatalf("data = %#v, want no missing refs", data)
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatalf("AllObjectIDs() error = %v", err)
	}
	sort.Strings(ids)
	if len(ids) != 1 || ids[0] != "archive/freya" {
		t.Fatalf("indexed IDs = %#v, want [archive/freya]", ids)
	}
}

func TestApplyChangeSetProjectsReusedMoveDestination(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("a.md", "A\n").
		WithFile("b.md", "B\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	indexPostMutationFiles(t, rt, "a.md", "b.md")

	if err := os.Rename(filepath.Join(v.Path, "b.md"), filepath.Join(v.Path, "c.md")); err != nil {
		t.Fatalf("move b to c: %v", err)
	}
	if err := os.Rename(filepath.Join(v.Path, "a.md"), filepath.Join(v.Path, "b.md")); err != nil {
		t.Fatalf("move a to b: %v", err)
	}

	changes := mutation.NewChangeSet()
	changes.AddMoved("b.md", "c.md")
	changes.AddMoved("a.md", "b.md")
	_, warnings := applyChangeSet(rt, changes)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatalf("AllObjectIDs() error = %v", err)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "b" || ids[1] != "c" {
		t.Fatalf("indexed IDs = %#v, want [b c]", ids)
	}
}

func TestApplyChangeSetProjectsRecreatedMoveSource(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("a.md", "Original A\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	indexPostMutationFiles(t, rt, "a.md")

	if err := os.Rename(filepath.Join(v.Path, "a.md"), filepath.Join(v.Path, "b.md")); err != nil {
		t.Fatalf("move a to b: %v", err)
	}
	if err := os.WriteFile(filepath.Join(v.Path, "a.md"), []byte("Recreated A\n"), 0o644); err != nil {
		t.Fatalf("recreate a: %v", err)
	}

	changes := mutation.NewChangeSet()
	changes.AddMoved("a.md", "b.md")
	changes.AddChanged("a.md")
	_, warnings := applyChangeSet(rt, changes)
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", warnings)
	}

	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatalf("AllObjectIDs() error = %v", err)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("indexed IDs = %#v, want [a b]", ids)
	}
}

func indexPostMutationFiles(t *testing.T, rt *vaultruntime.Runtime, relPaths ...string) {
	t.Helper()
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("open index: %v", err)
	}
	for _, relPath := range relPaths {
		filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
		content, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s: %v", relPath, err)
		}
		doc, err := parser.ParseDocumentWithOptions(string(content), filePath, rt.VaultPath, rt.ParseOptions)
		if err != nil {
			t.Fatalf("parse %s: %v", relPath, err)
		}
		if err := rt.DB.IndexDocument(doc, rt.Schema); err != nil {
			t.Fatalf("index %s: %v", relPath, err)
		}
	}
}
