package vaultwatch

import (
	"context"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestWatcherProjectsExternalEditAndSkipsOwnWrite(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	rt := testutil.NewRuntimeForTest(t, v.Path)
	if _, err := reindexsvc.Run(rt, reindexsvc.RunRequest{Full: true}); err != nil {
		t.Fatalf("reindex: %v", err)
	}
	rt.Close()

	before, err := index.ReadRevision(v.Path)
	if err != nil {
		t.Fatal(err)
	}

	w, err := Start(context.Background(), Options{
		VaultPath: v.Path,
		Debounce:  30 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	defer func() { _ = w.Close() }()

	v.WriteFile("people/alice.md", "---\ntype: person\nname: Freya\n---\n")
	deadline := time.Now().Add(2 * time.Second)
	var after index.Revision
	for {
		after, err = index.ReadRevision(v.Path)
		if err != nil {
			t.Fatal(err)
		}
		if after.Generation == before.Generation && after.Counter > before.Counter {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("revision did not move after external edit: before %#v after %#v", before, after)
		}
		time.Sleep(20 * time.Millisecond)
	}

	stable := after
	time.Sleep(80 * time.Millisecond)
	again, err := index.ReadRevision(v.Path)
	if err != nil {
		t.Fatal(err)
	}
	if again != stable {
		t.Fatalf("watcher reprojected an already-current file: %#v -> %#v", stable, again)
	}

	check, err := vaultruntime.New(v.Path, vaultruntime.Options{OpenDB: true})
	if err != nil {
		t.Fatal(err)
	}
	defer check.Close()
	obj, err := check.DB.GetObject("people/alice")
	if err != nil || obj == nil {
		t.Fatalf("alice missing: %v", err)
	}
	if name, _ := obj.Fields["name"].AsString(); name != "Freya" {
		t.Fatalf("name = %q, want Freya", name)
	}
}
