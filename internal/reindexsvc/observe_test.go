package reindexsvc

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func reindexFull(t *testing.T, vaultPath string) *RunResult {
	t.Helper()
	rt := testutil.NewRuntimeForTest(t, vaultPath)
	defer rt.Close()
	result, err := Run(rt, RunRequest{Full: true})
	if err != nil {
		t.Fatalf("reindex --full: %v", err)
	}
	return result
}

func openVaultDB(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{OpenDB: true})
}

func TestProjectChangesBumpsRevision(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	reindexFull(t, v.Path)
	rt := openVaultDB(t, v.Path)
	before, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}

	v.WriteFile("people/alice.md", "---\ntype: person\nname: Alice Updated\n---\n")
	changes := mutation.NewChangeSet()
	changes.AddChanged("people/alice.md")
	if result := ProjectChanges(rt, changes, ""); len(result.Warnings) != 0 {
		t.Fatalf("ProjectChanges warnings = %#v", result.Warnings)
	}
	after, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation != before.Generation {
		t.Fatalf("generation changed on write: %q -> %q", before.Generation, after.Generation)
	}
	if after.Counter <= before.Counter {
		t.Fatalf("counter = %d, want > %d", after.Counter, before.Counter)
	}
}

func TestFullReindexMintsNewGeneration(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	first := reindexFull(t, v.Path)
	if first.Revision.Generation == "" {
		t.Fatal("expected revision on first reindex")
	}
	second := reindexFull(t, v.Path)
	if second.Revision.Generation == "" || second.Revision.Generation == first.Revision.Generation {
		t.Fatalf("full reindex generation = %q, want a new value (was %q)", second.Revision.Generation, first.Revision.Generation)
	}
	if second.Data()["revision"] == nil {
		t.Fatal("reindex JSON omitted revision")
	}
}

func TestObserveFilesSkipsCurrentFingerprint(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	reindexFull(t, v.Path)
	rt := openVaultDB(t, v.Path)
	before, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if err := ObserveFiles(rt, ObserveRequest{Paths: []string{"people/alice.md"}}); err != nil {
		t.Fatalf("ObserveFiles: %v", err)
	}
	after, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Fatalf("own-write/current file reprojected: %#v -> %#v", before, after)
	}
}

func TestObserveFilesProjectsExternalEdit(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	reindexFull(t, v.Path)
	rt := openVaultDB(t, v.Path)
	before, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}
	v.WriteFile("people/alice.md", "---\ntype: person\nname: Freya\n---\n")
	if err := ObserveFiles(rt, ObserveRequest{Paths: []string{"people/alice.md"}}); err != nil {
		t.Fatalf("ObserveFiles: %v", err)
	}
	after, err := rt.DB.Revision()
	if err != nil {
		t.Fatal(err)
	}
	if after.Generation != before.Generation || after.Counter <= before.Counter {
		t.Fatalf("revision after external edit = %#v, before %#v", after, before)
	}
	obj, err := rt.DB.GetObject("people/alice")
	if err != nil {
		t.Fatal(err)
	}
	if obj == nil {
		t.Fatal("people/alice missing after observe")
	}
	if name, _ := obj.Fields["name"].AsString(); name != "Freya" {
		t.Fatalf("name = %q, want Freya", name)
	}
}

func TestObserveFilesConcurrentWriterDoesNotCorruptIndex(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		WithFile("people/bob.md", "---\ntype: person\nname: Bob\n---\n").
		Build()
	reindexFull(t, v.Path)

	var wg sync.WaitGroup
	errCh := make(chan error, 2)
	wg.Add(2)
	go func() {
		defer wg.Done()
		writer, err := vaultruntime.New(v.Path, vaultruntime.Options{OpenDB: true})
		if err != nil {
			errCh <- err
			return
		}
		defer writer.Close()
		for i := 0; i < 8; i++ {
			v.WriteFile("people/alice.md", "---\ntype: person\nname: Alice\n---\nwrite\n")
			changes := mutation.NewChangeSet()
			changes.AddChanged("people/alice.md")
			if result := ProjectChanges(writer, changes, ""); len(result.Warnings) != 0 {
				errCh <- result.Warnings[0].Err
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		observer, err := vaultruntime.New(v.Path, vaultruntime.Options{OpenDB: true})
		if err != nil {
			errCh <- err
			return
		}
		defer observer.Close()
		for i := 0; i < 8; i++ {
			v.WriteFile("people/bob.md", "---\ntype: person\nname: Bob\n---\nwatch\n")
			if err := ObserveFiles(observer, ObserveRequest{Paths: []string{"people/bob.md"}}); err != nil {
				errCh <- err
				return
			}
		}
	}()
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("concurrent projection: %v", err)
		}
	}

	check := openVaultDB(t, v.Path)
	if obj, err := check.DB.GetObject("people/alice"); err != nil || obj == nil {
		t.Fatalf("alice missing after concurrent writes: %v %#v", err, obj)
	}
	if obj, err := check.DB.GetObject("people/bob"); err != nil || obj == nil {
		t.Fatalf("bob missing after concurrent writes: %v %#v", err, obj)
	}
	if _, err := check.DB.Stats(); err != nil {
		t.Fatalf("stats after concurrent writes: %v", err)
	}
	check.Close()
	if _, err := Run(testutil.NewVaultRuntime(t, v.Path, vaultruntime.Options{}), RunRequest{}); err != nil {
		t.Fatalf("incremental reindex after concurrent writes: %v", err)
	}
}

func TestIndexDeletionMintsNewGeneration(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	first := reindexFull(t, v.Path)

	indexPath := filepath.Join(v.Path, ".raven", "index.db")
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(indexPath + suffix); err != nil && !os.IsNotExist(err) {
			t.Fatal(err)
		}
	}

	rev, err := index.ReadRevision(v.Path)
	if err != nil {
		t.Fatal(err)
	}
	if rev.Generation == "" || rev.Generation == first.Revision.Generation {
		t.Fatalf("generation after delete = %q, first %q", rev.Generation, first.Revision.Generation)
	}
	if rev.Counter != 0 {
		t.Fatalf("counter after delete = %d, want 0", rev.Counter)
	}
}
