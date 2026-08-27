package reindexsvc

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestProjectChangesAutoReindexDisabledJournalsChanges(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("auto_reindex: false\n").
		WithFile("people/alice.md", "---\ntype: person\nname: [\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	changes := mutation.NewChangeSet()
	changes.AddChanged("people/alice.md")

	result := ProjectChanges(rt, changes, "")
	if len(result.MissingRefs) != 0 || len(result.Warnings) != 0 {
		t.Fatalf("result = %#v, want deferred projection without annotations", result)
	}
	pending, err := indexjournal.Load(v.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if got := pending.Paths(); len(got) != 1 || got[0] != "people/alice.md" {
		t.Fatalf("pending paths = %#v, want people/alice.md", got)
	}
}

func TestProjectChangesReportsMissingRefsAcrossFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		WithFile("notes/one.md", "See [[people/ghost]].\n").
		WithFile("notes/two.md", "See [[projects/missing]].\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "people/alice.md")

	changes := mutation.NewChangeSet()
	changes.AddChanged("notes/one.md", "notes/two.md")
	result := ProjectChanges(rt, changes, "")
	if len(result.Warnings) != 0 || len(result.MissingRefs) != 2 {
		t.Fatalf("result = %#v, want two missing refs and no projection warning", result)
	}
	if result.MissingRefs[0].TargetPath != "people/ghost" || result.MissingRefs[1].TargetPath != "projects/missing" {
		t.Fatalf("missing refs = %#v, want sorted targets", result.MissingRefs)
	}
}

func TestProjectChangesProjectsMoveAndDelete(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/obsolete.md", "---\ntype: project\ntitle: Obsolete\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "people/freya.md", "projects/obsolete.md")
	if err := os.MkdirAll(filepath.Join(v.Path, "archive"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(v.Path, "people", "freya.md"), filepath.Join(v.Path, "archive", "freya.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(v.Path, "projects", "obsolete.md")); err != nil {
		t.Fatal(err)
	}
	changes := mutation.NewChangeSet()
	changes.AddMoved("people/freya.md", "archive/freya.md")
	changes.AddDeleted("projects/obsolete.md")

	result := ProjectChanges(rt, changes, "")
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none", result.Warnings)
	}
	pending, err := indexjournal.Load(v.Path)
	if err != nil || pending.Dirty() {
		t.Fatalf("journal = %#v, err = %v; want clean", pending, err)
	}
	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	if len(ids) != 1 || ids[0] != "archive/freya" {
		t.Fatalf("ids = %#v, want archive/freya", ids)
	}
}

func TestProjectChangesClearsOnlySuccessfullyProjectedFiles(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/good.md", "---\ntype: person\nname: Good\n---\n").
		WithFile("people/broken.md", "---\ntype: person\nname: [\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	changes := mutation.NewChangeSet()
	changes.AddChanged("people/good.md", "people/broken.md")

	result := ProjectChanges(rt, changes, "")
	if len(result.Warnings) != 1 || result.Warnings[0].Code != codes.WarnIndexUpdateFailed {
		t.Fatalf("warnings = %#v, want one index warning", result.Warnings)
	}
	pending, err := indexjournal.Load(v.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got := pending.Paths(); len(got) != 1 || got[0] != "people/broken.md" {
		t.Fatalf("pending = %#v, want people/broken.md", got)
	}
	if object, err := rt.DB.GetObject("people/good"); err != nil || object == nil {
		t.Fatalf("good object = %#v, err = %v", object, err)
	}
}

func TestProjectChangesRecordsFullRecoveryForResolutionFailure(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("source.md", "[[target]]\n").
		WithFile("target.md", "# Target\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "source.md")
	if _, err := rt.DB.DB().Exec(`
		CREATE TRIGGER fail_reference_resolution
		BEFORE UPDATE OF target_id ON refs
		BEGIN
			SELECT RAISE(ABORT, 'reference resolution failed');
		END;
	`); err != nil {
		t.Fatal(err)
	}
	changes := mutation.NewChangeSet()
	changes.AddChanged("target.md")

	result := ProjectChanges(rt, changes, "")
	if len(result.MissingRefs) != 0 || len(result.Warnings) != 1 {
		t.Fatalf("result = %#v, want one warning and no missing refs", result)
	}
	if result.Warnings[0].Code != codes.WarnRefResolutionIncomplete ||
		!strings.Contains(result.Warnings[0].Message, "vault-wide reference resolution did not complete") {
		t.Fatalf("warning = %#v", result.Warnings[0])
	}
	pending, err := indexjournal.Load(v.Path)
	if err != nil || !pending.Dirty() || !pending.RequiresFullScan() {
		t.Fatalf("pending = %#v, err = %v; want full scan", pending, err)
	}
}

func TestProjectChangesDoesNotRemoveConcurrentlyRecreatedPath(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "people/freya.md")
	filePath := filepath.Join(v.Path, "people", "freya.md")
	if err := os.Remove(filePath); err != nil {
		t.Fatal(err)
	}
	changes := mutation.NewChangeSet()
	changes.AddDeleted("people/freya.md")
	if err := os.WriteFile(filePath, []byte("---\ntype: person\nname: Frigg\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := ProjectChanges(rt, changes, "")
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	object, err := rt.DB.GetObject("people/freya")
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := object.Fields["name"].AsString(); !ok || got != "Frigg" {
		t.Fatalf("indexed name = %q, %v; want Frigg", got, ok)
	}
}

func TestProjectChangesProjectsReusedMoveDestination(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("a.md", "A\n").
		WithFile("b.md", "B\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "a.md", "b.md")
	if err := os.Rename(filepath.Join(v.Path, "b.md"), filepath.Join(v.Path, "c.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(v.Path, "a.md"), filepath.Join(v.Path, "b.md")); err != nil {
		t.Fatal(err)
	}
	changes := mutation.NewChangeSet()
	changes.AddMoved("b.md", "c.md")
	changes.AddMoved("a.md", "b.md")
	if result := ProjectChanges(rt, changes, ""); len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "b" || ids[1] != "c" {
		t.Fatalf("ids = %#v, want [b c]", ids)
	}
}

func TestProjectChangesProjectsRecreatedMoveSource(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("a.md", "Original A\n").
		Build()
	rt := testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{})
	projectTestPaths(t, rt, "a.md")
	if err := os.Rename(filepath.Join(v.Path, "a.md"), filepath.Join(v.Path, "b.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(v.Path, "a.md"), []byte("Recreated A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changes := mutation.NewChangeSet()
	changes.AddMoved("a.md", "b.md")
	changes.AddChanged("a.md")
	if result := ProjectChanges(rt, changes, ""); len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v", result.Warnings)
	}
	ids, err := rt.DB.AllObjectIDs()
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(ids)
	if len(ids) != 2 || ids[0] != "a" || ids[1] != "b" {
		t.Fatalf("ids = %#v, want [a b]", ids)
	}
}

func projectTestPaths(t *testing.T, rt *vaultruntime.Runtime, paths ...string) {
	t.Helper()
	changes := mutation.NewChangeSet()
	changes.AddChanged(paths...)
	if result := ProjectChanges(rt, changes, ""); len(result.Warnings) != 0 {
		t.Fatalf("seed projection warnings = %#v", result.Warnings)
	}
}
