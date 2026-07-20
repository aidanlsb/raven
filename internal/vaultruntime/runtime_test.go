package vaultruntime

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
