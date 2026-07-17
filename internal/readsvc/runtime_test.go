package readsvc

import (
	"os"
	"path/filepath"
	"testing"
)

// corruptSchemaYAML exists on disk but fails to parse, so schema.Load returns
// an error rather than the default schema returned for an absent file.
const corruptSchemaYAML = "version: 1\ntypes: [unterminated\n"

func writeCorruptSchemaVault(t *testing.T) string {
	t.Helper()
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(corruptSchemaYAML), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	return vaultPath
}

func TestNewRuntimeRequireSchemaFatalOnLoadFailure(t *testing.T) {
	t.Parallel()

	vaultPath := writeCorruptSchemaVault(t)

	rt, err := NewRuntime(vaultPath, RuntimeOptions{RequireSchema: true})
	if err == nil {
		if rt != nil {
			rt.Close()
		}
		t.Fatalf("NewRuntime(RequireSchema=true) succeeded, want fatal schema load error")
	}
}

func TestNewRuntimeDegradedRecordsSchemaLoadError(t *testing.T) {
	t.Parallel()

	vaultPath := writeCorruptSchemaVault(t)

	rt, err := NewRuntime(vaultPath, RuntimeOptions{})
	if err != nil {
		t.Fatalf("NewRuntime() returned fatal error in degraded mode: %v", err)
	}
	defer rt.Close()

	if rt.SchemaLoadErr == nil {
		t.Fatalf("SchemaLoadErr = nil, want recorded schema load failure")
	}
	if rt.Schema != nil {
		t.Fatalf("Schema = %#v, want nil after load failure", rt.Schema)
	}
}

func TestNewRuntimeMissingSchemaIsNotAFailure(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()

	rt, err := NewRuntime(vaultPath, RuntimeOptions{RequireSchema: true})
	if err != nil {
		t.Fatalf("NewRuntime() failed for missing schema.yaml: %v", err)
	}
	defer rt.Close()

	if rt.SchemaLoadErr != nil {
		t.Fatalf("SchemaLoadErr = %v, want nil for missing schema.yaml", rt.SchemaLoadErr)
	}
	if rt.Schema == nil {
		t.Fatalf("Schema = nil, want default schema for missing schema.yaml")
	}
}
