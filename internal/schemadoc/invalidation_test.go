package schemadoc

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemachange"
)

func writeMinimalSchema(t *testing.T, vaultPath string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 1\ntypes: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
}

func addTypeNamed(name string) func(*Document) error {
	return func(doc *Document) error {
		types := EnsureMap(doc.Root(), "types")
		types[name] = map[string]interface{}{
			"fields": map[string]interface{}{},
		}
		return nil
	}
}

func setTypeDescription(typeName, description string) func(*Document) error {
	return func(doc *Document) error {
		types := EnsureMap(doc.Root(), "types")
		typeNode := EnsureMap(types, typeName)
		typeNode["description"] = description
		return nil
	}
}

func TestEditWithInvalidationRequiresRecorder(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeMinimalSchema(t, vaultPath)
	before := mustReadSchema(t, vaultPath)

	_, err := EditWithInvalidation(vaultPath, nil, addTypeNamed("meeting"))
	if !errors.Is(err, ErrInvalidationRequired) {
		t.Fatalf("error = %v, want ErrInvalidationRequired", err)
	}
	if got := mustReadSchema(t, vaultPath); got != before {
		t.Fatalf("nil recorder rewrote schema.yaml")
	}
}

func TestEditWithInvalidationRecordsThroughExplicitPath(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeMinimalSchema(t, vaultPath)

	result, err := EditWithInvalidation(vaultPath, schemachange.RecordInvalidation, addTypeNamed("meeting"))
	if err != nil {
		t.Fatalf("EditWithInvalidation: %v", err)
	}
	if !result.Changed {
		t.Fatal("expected schema.yaml to change")
	}
	if result.OperationID == "" {
		t.Fatal("expected journal operation ID for a type addition")
	}
	if result.Classification.Policy != schemachange.PolicyFullScan {
		t.Fatalf("policy = %q, want %q", result.Classification.Policy, schemachange.PolicyFullScan)
	}

	journal, err := indexjournal.Load(vaultPath)
	if err != nil {
		t.Fatalf("load journal: %v", err)
	}
	if !journal.Dirty() || !journal.RequiresFullScan() {
		t.Fatalf("journal dirty=%v full_scan=%v, want stale full scan", journal.Dirty(), journal.RequiresFullScan())
	}
}

func TestEditWithInvalidation_TwoEditsDoNotShareClassification(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeMinimalSchema(t, vaultPath)

	first, err := EditWithInvalidation(vaultPath, schemachange.RecordInvalidation, addTypeNamed("meeting"))
	if err != nil {
		t.Fatalf("first edit: %v", err)
	}
	second, err := EditWithInvalidation(vaultPath, schemachange.RecordInvalidation, setTypeDescription("meeting", "Calendar events"))
	if err != nil {
		t.Fatalf("second edit: %v", err)
	}

	if first.Classification.Policy != schemachange.PolicyFullScan {
		t.Fatalf("first policy = %q, want %q", first.Classification.Policy, schemachange.PolicyFullScan)
	}
	if second.Classification.Policy != schemachange.PolicyNone {
		t.Fatalf("second policy = %q, want %q", second.Classification.Policy, schemachange.PolicyNone)
	}
	if first.Classification.Policy == second.Classification.Policy {
		t.Fatal("edits shared classification")
	}
}

func TestEditWithInvalidation_ConcurrentEditsKeepOwnClassification(t *testing.T) {
	t.Parallel()

	type outcome struct {
		result *EditResult
		err    error
	}

	firstVault := t.TempDir()
	secondVault := t.TempDir()
	writeMinimalSchema(t, firstVault)
	if err := os.WriteFile(filepath.Join(secondVault, "schema.yaml"), []byte("version: 1\ntypes:\n  meeting:\n    fields: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write second schema: %v", err)
	}

	results := make([]outcome, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		result, err := EditWithInvalidation(firstVault, schemachange.RecordInvalidation, addTypeNamed("meeting"))
		results[0] = outcome{result: result, err: err}
	}()
	go func() {
		defer wg.Done()
		result, err := EditWithInvalidation(secondVault, schemachange.RecordInvalidation, setTypeDescription("meeting", "Calendar events"))
		results[1] = outcome{result: result, err: err}
	}()
	wg.Wait()

	for i, got := range results {
		if got.err != nil {
			t.Fatalf("edit %d: %v", i, got.err)
		}
	}
	if results[0].result.Classification.Policy != schemachange.PolicyFullScan {
		t.Fatalf("first policy = %q, want %q", results[0].result.Classification.Policy, schemachange.PolicyFullScan)
	}
	if results[1].result.Classification.Policy != schemachange.PolicyNone {
		t.Fatalf("second policy = %q, want %q", results[1].result.Classification.Policy, schemachange.PolicyNone)
	}
}

func mustReadSchema(t *testing.T, vaultPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(vaultPath, "schema.yaml"))
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	return string(data)
}

func TestEditWithInvalidation_RecorderFailureDoesNotWrite(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	writeMinimalSchema(t, vaultPath)
	before := mustReadSchema(t, vaultPath)
	wantErr := errors.New("journal write failed")

	_, err := EditWithInvalidation(vaultPath, func(string, *schema.Schema, *schema.Schema) (string, schemachange.Classification, error) {
		return "", schemachange.Classification{}, wantErr
	}, addTypeNamed("meeting"))
	if !errors.Is(err, wantErr) {
		t.Fatalf("error = %v, want %v", err, wantErr)
	}
	if got := mustReadSchema(t, vaultPath); got != before {
		t.Fatalf("failed recorder rewrote schema.yaml")
	}
}
