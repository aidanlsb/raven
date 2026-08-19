package vaultruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestFromRequest(t *testing.T) {
	t.Parallel()

	existing := &Runtime{VaultPath: "existing"}
	got, constructed := FromRequest(existing, "ignored", nil, nil, nil)
	if got != existing {
		t.Fatalf("FromRequest() runtime = %p, want existing runtime %p", got, existing)
	}
	if constructed {
		t.Fatal("FromRequest() constructed = true, want false for existing runtime")
	}

	vaultCfg := &config.VaultConfig{}
	sch := &schema.Schema{}
	parseOptions := &parser.ParseOptions{}
	got, constructed = FromRequest(nil, "vault", vaultCfg, sch, parseOptions)
	if !constructed {
		t.Fatal("FromRequest() constructed = false, want true for new runtime")
	}
	if got.VaultPath != "vault" {
		t.Fatalf("FromRequest() VaultPath = %q, want %q", got.VaultPath, "vault")
	}
	if got.VaultCfg != vaultCfg {
		t.Fatal("FromRequest() did not preserve VaultCfg")
	}
	if got.Schema != sch {
		t.Fatal("FromRequest() did not preserve Schema")
	}
	if got.ParseOptions != parseOptions {
		t.Fatal("FromRequest() did not preserve ParseOptions")
	}
}

func TestRequire(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		runtime *Runtime
		wantErr bool
	}{
		{name: "nil runtime", runtime: nil, wantErr: true},
		{name: "empty path", runtime: &Runtime{}, wantErr: true},
		{name: "whitespace path", runtime: &Runtime{VaultPath: " \t\n"}, wantErr: true},
		{name: "vault path", runtime: &Runtime{VaultPath: "/tmp/vault"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := Require(tt.runtime)
			if tt.wantErr {
				if !errors.Is(err, ErrVaultPathRequired) {
					t.Fatalf("Require() error = %v, want ErrVaultPathRequired", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Require() error = %v, want nil", err)
			}
		})
	}
}

func TestRequirePath(t *testing.T) {
	t.Parallel()

	if err := RequirePath(" \t\n"); !errors.Is(err, ErrVaultPathRequired) {
		t.Fatalf("RequirePath() error = %v, want ErrVaultPathRequired", err)
	}
	if err := RequirePath("/tmp/vault"); err != nil {
		t.Fatalf("RequirePath() error = %v, want nil", err)
	}
}

func TestNewBuildsInvocationScopedDependencies(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	configYAML := "directories:\n  type: type/\n  page: page/\n  daily: journal/\n"
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte(configYAML), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	rt, err := New(vaultPath, Options{RequireSchema: true, OpenDB: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer rt.Close()

	if rt.Schema == nil {
		t.Fatal("Schema = nil, want default schema")
	}
	if rt.DB == nil {
		t.Fatal("DB = nil, want open database")
	}
	if rt.ParseOptions == nil {
		t.Fatal("ParseOptions = nil")
	}
	if got, want := rt.ParseOptions.ObjectsRoot, "type/"; got != want {
		t.Fatalf("ObjectsRoot = %q, want %q", got, want)
	}
	if got, want := rt.ParseOptions.PagesRoot, "page/"; got != want {
		t.Fatalf("PagesRoot = %q, want %q", got, want)
	}
	if got, want := rt.ParseOptions.DailyRoot, "journal"; got != want {
		t.Fatalf("DailyRoot = %q, want %q", got, want)
	}
}

func TestNewSchemaFailurePolicy(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	t.Run("required", func(t *testing.T) {
		rt, err := New(vaultPath, Options{RequireSchema: true})
		if rt != nil {
			rt.Close()
		}
		var setupErr *SetupError
		if !errors.As(err, &setupErr) || setupErr.Stage != StageSchema {
			t.Fatalf("error = %v, want schema SetupError", err)
		}
	})

	t.Run("degraded", func(t *testing.T) {
		rt, err := New(vaultPath, Options{})
		if err != nil {
			t.Fatalf("New() error: %v", err)
		}
		defer rt.Close()
		if rt.SchemaLoadErr == nil {
			t.Fatal("SchemaLoadErr = nil, want recorded failure")
		}
		if rt.Schema != nil {
			t.Fatalf("Schema = %#v, want nil", rt.Schema)
		}
	})
}

func TestOpenDBIsIdempotent(t *testing.T) {
	t.Parallel()

	rt, err := New(t.TempDir(), Options{})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer rt.Close()

	if err := rt.OpenDB(); err != nil {
		t.Fatalf("first OpenDB() error: %v", err)
	}
	first := rt.DB
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("second OpenDB() error: %v", err)
	}
	if rt.DB != first {
		t.Fatal("OpenDB() replaced the existing database handle")
	}
}

func TestCloseClearsDatabaseAndAllowsReopen(t *testing.T) {
	t.Parallel()

	rt, err := New(t.TempDir(), Options{OpenDB: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	first := rt.DB
	rt.Close()
	if rt.DB != nil {
		t.Fatal("Close() left DB attached")
	}
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("OpenDB() after Close() error: %v", err)
	}
	defer rt.Close()
	if rt.DB == nil || rt.DB == first {
		t.Fatal("OpenDB() did not attach a fresh database handle")
	}
}

func TestNewCanSkipUnneededSchema(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	rt, err := New(vaultPath, Options{SkipSchema: true})
	if err != nil {
		t.Fatalf("New() error: %v", err)
	}
	defer rt.Close()
	if rt.VaultCfg == nil {
		t.Fatal("VaultCfg = nil")
	}
	if rt.Schema != nil || rt.SchemaLoadErr != nil {
		t.Fatalf("schema was unexpectedly loaded: schema=%#v err=%v", rt.Schema, rt.SchemaLoadErr)
	}
}

func TestNewSchemaFirstPreservesFailurePrecedence(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("types: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("directories: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	_, err := New(vaultPath, Options{SchemaFirst: true, RequireSchema: true})
	var setupErr *SetupError
	if !errors.As(err, &setupErr) || setupErr.Stage != StageSchema {
		t.Fatalf("error = %v, want schema SetupError", err)
	}
}
