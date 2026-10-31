package readsvc

import (
	"database/sql"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexschema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestShouldRefresh(t *testing.T) {
	t.Parallel()

	enabled := false
	disabledCfg := &config.VaultConfig{AutoReindex: &enabled}

	trueVal := true
	falseVal := false

	tests := []struct {
		name      string
		rt        *vaultruntime.Runtime
		noRefresh bool
		refresh   *bool
		want      bool
	}{
		{name: "default on", rt: &vaultruntime.Runtime{VaultCfg: &config.VaultConfig{}}, want: true},
		{name: "nil runtime defaults on", want: true},
		{name: "auto_reindex false", rt: &vaultruntime.Runtime{VaultCfg: disabledCfg}, want: false},
		{name: "no-refresh wins", rt: &vaultruntime.Runtime{VaultCfg: &config.VaultConfig{}}, noRefresh: true, want: false},
		{name: "explicit refresh true overrides auto_reindex false", rt: &vaultruntime.Runtime{VaultCfg: disabledCfg}, refresh: &trueVal, want: true},
		{name: "explicit refresh false skips", rt: &vaultruntime.Runtime{VaultCfg: &config.VaultConfig{}}, refresh: &falseVal, want: false},
		{name: "no-refresh wins over explicit refresh", rt: &vaultruntime.Runtime{VaultCfg: &config.VaultConfig{}}, noRefresh: true, refresh: &trueVal, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldRefresh(tt.rt, tt.noRefresh, tt.refresh); got != tt.want {
				t.Fatalf("ShouldRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestEnsureIndexFreshIndexesNewEditedAndDeletedFiles(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	t.Cleanup(rt.Close)

	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("initial EnsureIndexFresh: %v", err)
	}
	if obj, err := rt.DB.GetObject("people/alice"); err != nil || obj == nil {
		t.Fatalf("alice missing after initial refresh: %v %#v", err, obj)
	}

	bobPath := filepath.Join(vault.Path, "people", "bob.md")
	if err := os.WriteFile(bobPath, []byte("---\ntype: person\nname: Bob\n---\n"), 0o644); err != nil {
		t.Fatalf("write bob: %v", err)
	}
	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("refresh after new file: %v", err)
	}
	if obj, err := rt.DB.GetObject("people/bob"); err != nil || obj == nil {
		t.Fatalf("bob missing after new-file refresh: %v %#v", err, obj)
	}

	alicePath := filepath.Join(vault.Path, "people", "alice.md")
	if err := os.WriteFile(alicePath, []byte("---\ntype: person\nname: Alicia\n---\n"), 0o644); err != nil {
		t.Fatalf("edit alice: %v", err)
	}
	future := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(alicePath, future, future); err != nil {
		t.Fatalf("bump alice mtime: %v", err)
	}
	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("refresh after edit: %v", err)
	}
	obj, err := rt.DB.GetObject("people/alice")
	if err != nil || obj == nil {
		t.Fatalf("alice missing after edit refresh: %v %#v", err, obj)
	}
	if got, ok := obj.Fields["name"].AsString(); !ok || got != "Alicia" {
		t.Fatalf("alice name = %q, %v; want Alicia", got, ok)
	}

	if err := os.Remove(bobPath); err != nil {
		t.Fatalf("remove bob: %v", err)
	}
	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("refresh after delete: %v", err)
	}
	if obj, err := rt.DB.GetObject("people/bob"); err != nil {
		t.Fatalf("get deleted bob: %v", err)
	} else if obj != nil {
		t.Fatalf("bob still indexed after delete refresh: %#v", obj)
	}
}

func TestEnsureIndexFreshRebuildsIncompatibleSchema(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", "---\ntype: person\nname: Alice\n---\n").
		Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("initial EnsureIndexFresh: %v", err)
	}
	rt.Close()

	db, err := sql.Open("sqlite", filepath.Join(vault.Path, ".raven", "index.db"))
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if _, err := db.Exec(`UPDATE meta SET value = ? WHERE key = 'version'`, strconv.Itoa(indexschema.CurrentDBVersion-1)); err != nil {
		_ = db.Close()
		t.Fatalf("downgrade index version: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	rt = testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{})
	t.Cleanup(rt.Close)
	if _, err := EnsureIndexFresh(rt); err != nil {
		t.Fatalf("EnsureIndexFresh after schema mismatch: %v", err)
	}
	if rt.DB == nil {
		t.Fatal("expected reopened database after rebuild")
	}
	if obj, err := rt.DB.GetObject("people/alice"); err != nil || obj == nil {
		t.Fatalf("alice missing after schema rebuild: %v %#v", err, obj)
	}
}

func TestEnsureIndexFreshNoChangeIsCheapWalk(t *testing.T) {
	t.Parallel()

	const n = 400
	builder := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema())
	for i := 0; i < n; i++ {
		rel := filepath.ToSlash(filepath.Join("people", "p"+strconv.Itoa(i)+".md"))
		builder = builder.WithFile(rel, "---\ntype: person\nname: P"+strconv.Itoa(i)+"\n---\n")
	}
	vault := builder.Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	t.Cleanup(rt.Close)

	start := time.Now()
	first, err := EnsureIndexFresh(rt)
	if err != nil {
		t.Fatalf("initial EnsureIndexFresh: %v", err)
	}
	indexDuration := time.Since(start)
	if first.Indexed < n {
		t.Fatalf("initial indexed = %d, want at least %d", first.Indexed, n)
	}

	start = time.Now()
	second, err := EnsureIndexFresh(rt)
	if err != nil {
		t.Fatalf("no-change EnsureIndexFresh: %v", err)
	}
	noChangeDuration := time.Since(start)
	if second.Indexed != 0 {
		t.Fatalf("no-change indexed = %d, want 0", second.Indexed)
	}
	t.Logf("indexed %d files in %s; no-change refresh in %s", n, indexDuration, noChangeDuration)
}

func TestCanRefresh(t *testing.T) {
	t.Parallel()
	if CanRefresh(nil) {
		t.Fatal("nil runtime should not refresh")
	}
	if CanRefresh(&vaultruntime.Runtime{VaultCfg: &config.VaultConfig{}}) {
		t.Fatal("schema-less runtime should not refresh")
	}
}

func TestIsRebuildRequired(t *testing.T) {
	t.Parallel()
	if !isRebuildRequired(index.ErrIndexRebuildRequired) {
		t.Fatal("expected ErrIndexRebuildRequired to be a rebuild signal")
	}
	wrapped := &vaultruntime.SetupError{
		Stage:   vaultruntime.StageDatabase,
		Failure: vaultruntime.SetupFailureIndexRebuildRequired,
		Err:     index.ErrIndexRebuildRequired,
	}
	if !isRebuildRequired(wrapped) {
		t.Fatal("expected SetupError rebuild-required to be a rebuild signal")
	}
}
