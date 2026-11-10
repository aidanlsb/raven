package vaultsession

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const corruptSchemaYAML = "version: 1\ntypes: [unterminated\n"

func TestOpenEmptyPathIsVaultNotSpecified(t *testing.T) {
	t.Parallel()

	_, err := Open("", Required())
	assertServiceCode(t, err, codes.ErrVaultNotSpecified)
}

func TestOpenRequiredFatalOnCorruptSchema(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	vault.WriteFile("schema.yaml", corruptSchemaYAML)

	_, err := Open(vault.Path, Required())
	assertServiceCode(t, err, codes.ErrSchemaInvalid)
}

func TestOpenConfigToleratesCorruptSchema(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		Build()
	vault.WriteFile("schema.yaml", corruptSchemaYAML)

	sess, err := Open(vault.Path, Config())
	if err != nil {
		t.Fatalf("Open(Config) = %v, want success with degraded schema", err)
	}
	defer sess.Close()
	if sess.Runtime().SchemaLoadErr == nil {
		t.Fatal("SchemaLoadErr is nil, want recorded load failure")
	}
	if _, ok := sess.SchemaLoadWarning(); !ok {
		t.Fatal("SchemaLoadWarning() missing")
	}
}

func TestOpenRequiredDBOpensIndex(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	sess, err := Open(vault.Path, RequiredDB())
	if err != nil {
		t.Fatalf("Open(RequiredDB) = %v", err)
	}
	defer sess.Close()
	if sess.Runtime().DB == nil {
		t.Fatal("RequiredDB left the index closed")
	}
}

func TestOpenConfigOnlySkipsSchema(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(corruptSchemaYAML).
		Build()

	sess, err := Open(vault.Path, ConfigOnly())
	if err != nil {
		t.Fatalf("Open(ConfigOnly) = %v, want skip of corrupt schema", err)
	}
	defer sess.Close()
	if sess.Runtime().SchemaLoadErr != nil {
		t.Fatalf("ConfigOnly recorded SchemaLoadErr = %v", sess.Runtime().SchemaLoadErr)
	}
}

func TestOpenLazySkipsConfigAndSchema(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithRavenYAML(":::not-yaml").
		WithSchema(corruptSchemaYAML).
		Build()

	sess, err := Open(vault.Path, Lazy())
	if err != nil {
		t.Fatalf("Open(Lazy) = %v", err)
	}
	defer sess.Close()
	if sess.Runtime().VaultCfg != nil {
		t.Fatal("Lazy loaded raven.yaml")
	}
}

func TestOpenSchemaMutationRetriesInvalidConfig(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(":::not-yaml").
		Build()

	sess, err := Open(vault.Path, SchemaMutation())
	if err != nil {
		t.Fatalf("Open(SchemaMutation) = %v, want config failure to be non-fatal", err)
	}
	defer sess.Close()
	if sess.Runtime().DB == nil {
		t.Fatal("SchemaMutation left the index closed")
	}
}

func TestOpenInvalidConfigIsConfigInvalid(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML(":::not-yaml").
		Build()

	_, err := Open(vault.Path, Required())
	assertServiceCode(t, err, codes.ErrConfigInvalid)
}

func TestOpenReadRefreshesAndOpensDB(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nBody.\n").
		Build()

	sess, err := Open(vault.Path, Read(RefreshPolicy{}))
	if err != nil {
		t.Fatalf("Open(Read) = %v", err)
	}
	defer sess.Close()
	if sess.Runtime().DB == nil {
		t.Fatal("Read left the index closed")
	}
}

func TestOpenReadNoRefreshSkipsEnsure(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nBody.\n").
		Build()

	sess, err := Open(vault.Path, Read(RefreshPolicy{NoRefresh: true}))
	if err != nil {
		t.Fatalf("Open(Read no-refresh) = %v", err)
	}
	defer sess.Close()
	if sess.Runtime().DB == nil {
		t.Fatal("Read(no-refresh) should still open the index")
	}
}

func TestMutateBeginsWriteAheadAndProjects(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nOld body.\n").
		Build()
	sess, err := Open(vault.Path, Read(RefreshPolicy{}))
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer sess.Close()

	proj, err := sess.Mutate(func(rt *vaultruntime.Runtime) (mutation.ChangeSet, error) {
		path := filepath.Join(rt.VaultPath, "projects/roadmap.md")
		if writeErr := os.WriteFile(path, []byte("---\ntype: project\ntitle: Roadmap\nstatus: active\n---\n\nNew body.\n"), 0o644); writeErr != nil {
			return mutation.ChangeSet{}, writeErr
		}
		changes := mutation.NewChangeSet()
		changes.AddChanged("projects/roadmap.md")
		return changes, nil
	})
	if err != nil {
		t.Fatalf("Mutate = %v", err)
	}
	if len(proj.MissingRefs) != 0 {
		t.Fatalf("missing refs = %#v", proj.MissingRefs)
	}
	snapshot, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if snapshot.Dirty() {
		t.Fatalf("journal still dirty after successful Mutate: %#v", snapshot)
	}
}

func TestMutateAbandonPreservesUnknownGuard(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	sess, err := Open(vault.Path, Required())
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer sess.Close()

	_, err = sess.Mutate(func(*vaultruntime.Runtime) (mutation.ChangeSet, error) {
		return mutation.ChangeSet{}, errors.New("mutation failed")
	})
	if err == nil || err.Error() != "mutation failed" {
		t.Fatalf("Mutate err = %v, want mutation failed", err)
	}
	snapshot, err := indexjournal.Load(vault.Path)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !snapshot.Dirty() || !snapshot.RequiresFullScan() {
		t.Fatalf("snapshot = %#v, want recoverable unknown guard", snapshot)
	}
	if err := indexjournal.CompleteRecoveredUnknown(vault.Path, snapshot); err != nil {
		t.Fatalf("complete abandoned guard: %v", err)
	}
}

func TestMutateFailsClosedWhenJournalBeginFails(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	sess, err := Open(vault.Path, Required())
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer sess.Close()

	ravenDir := filepath.Join(vault.Path, ".raven")
	if err := os.RemoveAll(ravenDir); err != nil {
		t.Fatalf("remove .raven: %v", err)
	}
	if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write blocking .raven file: %v", err)
	}

	_, err = sess.Mutate(func(*vaultruntime.Runtime) (mutation.ChangeSet, error) {
		t.Fatal("fn should not run when journal begin fails")
		return mutation.ChangeSet{}, nil
	})
	assertServiceCode(t, err, codes.ErrDatabase)
	if err == nil || !strings.Contains(err.Error(), "failed to establish index recovery guard") {
		t.Fatalf("error = %v, want journal begin message", err)
	}
}

func TestApplyReturnsFnValue(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	sess, err := Open(vault.Path, Required())
	if err != nil {
		t.Fatalf("Open = %v", err)
	}
	defer sess.Close()

	value, _, err := Apply(sess, func(*vaultruntime.Runtime) (string, mutation.ChangeSet, error) {
		return "ok", mutation.NewChangeSet(), nil
	})
	if err != nil {
		t.Fatalf("Apply = %v", err)
	}
	if value != "ok" {
		t.Fatalf("value = %q, want ok", value)
	}
}

func TestMissingRefWarningInferredType(t *testing.T) {
	t.Parallel()
	warning := missingRefWarning(&check.MissingRef{TargetPath: "people/ghost", InferredType: "person"})
	if warning.Code != codes.WarnRefTargetMissing || warning.SuggestedType != "person" {
		t.Fatalf("warning = %#v", warning)
	}
	if warning.CreateInvoke == nil || warning.CreateInvoke.Command != "new" {
		t.Fatalf("create invoke = %#v, want new", warning.CreateInvoke)
	}
	if warning.CreateInvoke.Args["path"] != "people/ghost" || warning.CreateInvoke.Args["title"] != "ghost" {
		t.Fatalf("create args = %#v", warning.CreateInvoke.Args)
	}
}

func TestMissingRefWarningUnknownTypeFallsBackToCheck(t *testing.T) {
	t.Parallel()
	warning := missingRefWarning(&check.MissingRef{TargetPath: "misc/thing"})
	if warning.Code != codes.WarnRefTargetMissing || warning.SuggestedType != "" {
		t.Fatalf("warning = %#v", warning)
	}
	if warning.CreateInvoke == nil || warning.CreateInvoke.Command != "check create-missing" {
		t.Fatalf("create invoke = %#v, want check create-missing", warning.CreateInvoke)
	}
}

func assertServiceCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("error = %#v, want svcerr %q", err, want)
	}
	if svcErr.Code != want {
		t.Fatalf("code = %q, want %q (err=%v)", svcErr.Code, want, err)
	}
}
