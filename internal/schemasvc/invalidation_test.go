package schemasvc

import (
	"errors"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func TestAddType_MarksIndexStaleThroughExplicitPath(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("auto_reindex: false\n").
		Build()

	result, err := AddType(schemaTestRuntime(t, vault.Path), AddTypeRequest{TypeName: "meeting"})
	if err != nil {
		t.Fatalf("AddType: %v", err)
	}
	if len(result.Warnings) != 0 {
		t.Fatalf("warnings = %#v, want none when auto-reindex is off", result.Warnings)
	}

	journal, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() || !journal.RequiresFullScan() {
		t.Fatalf("journal dirty=%v full_scan=%v, want stale full scan", journal.Dirty(), journal.RequiresFullScan())
	}
}

func TestEditRuntimeSchema_RefreshFailureReturnsWarning(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	wantErr := errors.New("refresh boom")

	warnings, err := editRuntimeSchemaRefreshing(rt, "Run 'rvn init' first", codes.ErrSchemaNotFound, func(doc *schemadoc.Document) error {
		types := schemadoc.EnsureMap(doc.Root(), "types")
		types["meeting"] = map[string]interface{}{"fields": map[string]interface{}{}}
		return nil
	}, func(*vaultruntime.Runtime) error { return wantErr })
	if err != nil {
		t.Fatalf("editRuntimeSchemaRefreshing: %v", err)
	}
	if len(warnings) != 1 {
		t.Fatalf("warnings = %#v, want 1", warnings)
	}
	if warnings[0].Code != codes.WarnIndexUpdateFailed {
		t.Fatalf("warning code = %q, want %q", warnings[0].Code, codes.WarnIndexUpdateFailed)
	}
	if warnings[0].Ref != reindexsvc.IndexUpdateFailedWarningRef {
		t.Fatalf("warning ref = %q, want %q", warnings[0].Ref, reindexsvc.IndexUpdateFailedWarningRef)
	}
	if warnings[0].Message == "" {
		t.Fatal("warning message is empty")
	}

	journal, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() {
		t.Fatal("expected journal to stay dirty after a failed refresh")
	}
}

func TestEditRuntimeSchema_RefreshSuccessHasNoWarning(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.MinimalSchema()).Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	reindexCalled := false

	warnings, err := editRuntimeSchemaRefreshing(rt, "Run 'rvn init' first", codes.ErrSchemaNotFound, func(doc *schemadoc.Document) error {
		types := schemadoc.EnsureMap(doc.Root(), "types")
		types["meeting"] = map[string]interface{}{"fields": map[string]interface{}{}}
		return nil
	}, func(*vaultruntime.Runtime) error {
		reindexCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("editRuntimeSchemaRefreshing: %v", err)
	}
	if !reindexCalled {
		t.Fatal("expected refresh to run when auto-reindex is enabled")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none after a successful refresh", warnings)
	}
}

func TestEditRuntimeSchema_AutoReindexOffWritesJournalWithoutWarning(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("auto_reindex: false\n").
		Build()
	rt := testutil.NewVaultRuntime(t, vault.Path, vaultruntime.Options{OpenDB: true})
	reindexCalled := false

	warnings, err := editRuntimeSchemaRefreshing(rt, "Run 'rvn init' first", codes.ErrSchemaNotFound, func(doc *schemadoc.Document) error {
		types := schemadoc.EnsureMap(doc.Root(), "types")
		types["meeting"] = map[string]interface{}{"fields": map[string]interface{}{}}
		return nil
	}, func(*vaultruntime.Runtime) error {
		reindexCalled = true
		return errors.New("should not run")
	})
	if err != nil {
		t.Fatalf("editRuntimeSchemaRefreshing: %v", err)
	}
	if reindexCalled {
		t.Fatal("refresh ran with auto-reindex off")
	}
	if len(warnings) != 0 {
		t.Fatalf("warnings = %#v, want none when auto-reindex is off", warnings)
	}

	journal, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() {
		t.Fatal("expected journal to stay dirty when auto-reindex is off")
	}
}
