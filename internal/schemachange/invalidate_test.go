package schemachange

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestRecordInvalidation_PolicyNone(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()

	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Description: "Old description",
		Fields:      map[string]*schema.FieldDefinition{},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Description: "New description",
		Fields:      map[string]*schema.FieldDefinition{},
	}

	operationID, classification, err := RecordInvalidation(vaultPath, before, after)
	if err != nil {
		t.Fatalf("RecordInvalidation error: %v", err)
	}
	if operationID != "" {
		t.Errorf("expected no operation ID for PolicyNone, got %q", operationID)
	}
	if classification.Policy != PolicyNone {
		t.Errorf("expected PolicyNone, got %v", classification.Policy)
	}

	// Verify no journal entry was created
	journal, loadErr := indexjournal.Load(vaultPath)
	if loadErr != nil {
		t.Fatalf("load journal: %v", loadErr)
	}
	if journal.Dirty() {
		t.Errorf("expected clean journal for PolicyNone, got %v operations", len(journal.Operations))
	}
}

func TestRecordInvalidation_FullScan(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()

	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
		},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString},
			"email": {Type: schema.FieldTypeString},
		},
	}

	operationID, classification, err := RecordInvalidation(vaultPath, before, after)
	if err != nil {
		t.Fatalf("RecordInvalidation error: %v", err)
	}
	if operationID == "" {
		t.Error("expected operation ID for FullScan")
	}
	if classification.Policy != PolicyFullScan {
		t.Errorf("expected PolicyFullScan, got %v", classification.Policy)
	}

	// Verify journal entry was created
	journal, loadErr := indexjournal.Load(vaultPath)
	if loadErr != nil {
		t.Fatalf("load journal: %v", loadErr)
	}
	if !journal.Dirty() {
		t.Error("expected dirty journal for PolicyFullScan")
	}
	if !journal.RequiresFullScan() {
		t.Error("expected journal to require full scan")
	}
}

func TestRecordInvalidation_ResolverRefresh(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()

	before := schema.New()
	before.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "people/",
		Fields:      map[string]*schema.FieldDefinition{},
	}
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		DefaultPath: "person/",
		Fields:      map[string]*schema.FieldDefinition{},
	}

	operationID, classification, err := RecordInvalidation(vaultPath, before, after)
	if err != nil {
		t.Fatalf("RecordInvalidation error: %v", err)
	}
	if operationID == "" {
		t.Error("expected operation ID for ResolverRefresh")
	}
	if classification.Policy != PolicyResolverRefresh {
		t.Errorf("expected PolicyResolverRefresh, got %v", classification.Policy)
	}

	// Verify journal entry was created
	journal, loadErr := indexjournal.Load(vaultPath)
	if loadErr != nil {
		t.Fatalf("load journal: %v", loadErr)
	}
	if !journal.Dirty() {
		t.Error("expected dirty journal for PolicyResolverRefresh")
	}
}

func TestApplyInvalidation_AutoReindexDisabled(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("person/alice.md", "---\ntype: person\nname: Alice\n---\n# Alice\n").
		WithRavenYAML("auto_reindex: false\n").
		Build()

	// Record invalidation
	before, _ := schema.Load(vault.Path)
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString, Required: true},
			"email": {Type: schema.FieldTypeString},
		},
	}

	operationID, classification, err := RecordInvalidation(vault.Path, before, after)
	if err != nil {
		t.Fatalf("RecordInvalidation error: %v", err)
	}

	// Apply invalidation with auto-reindex disabled
	rt := testutil.NewRuntimeForTest(t, vault.Path)
	defer rt.Close()

	applyErr := ApplyInvalidation(rt, operationID, classification)
	if applyErr != nil {
		t.Fatalf("ApplyInvalidation error: %v", applyErr)
	}

	// Verify journal entry persists (not cleared because auto-reindex is disabled)
	journal, loadErr := indexjournal.Load(vault.Path)
	if loadErr != nil {
		t.Fatalf("load journal: %v", loadErr)
	}
	if !journal.Dirty() {
		t.Error("expected journal to remain dirty when auto-reindex is disabled")
	}
}

func TestApplyInvalidation_AutoReindexEnabled(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("person/alice.md", "---\ntype: person\nname: Alice\n---\n# Alice\n").
		Build()

	// Ensure vault has index
	rt := testutil.NewRuntimeForTest(t, vault.Path)
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("open db: %v", err)
	}
	rt.Close()

	// Record invalidation
	before, _ := schema.Load(vault.Path)
	after := schema.New()
	after.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":  {Type: schema.FieldTypeString, Required: true},
			"email": {Type: schema.FieldTypeString},
		},
	}

	operationID, classification, err := RecordInvalidation(vault.Path, before, after)
	if err != nil {
		t.Fatalf("RecordInvalidation error: %v", err)
	}

	// Apply invalidation with auto-reindex enabled
	rt = testutil.NewRuntimeForTest(t, vault.Path)
	defer rt.Close()
	if err := rt.OpenDB(); err != nil {
		t.Fatalf("open db for apply: %v", err)
	}

	applyErr := ApplyInvalidation(rt, operationID, classification)
	if applyErr != nil {
		t.Fatalf("ApplyInvalidation error: %v", applyErr)
	}

	// Verify journal entry was cleared (recovered by auto-reindex)
	journal, loadErr := indexjournal.Load(vault.Path)
	if loadErr != nil {
		t.Fatalf("load journal: %v", loadErr)
	}
	if journal.Dirty() {
		t.Error("expected journal to be clean after auto-reindex")
	}
}

func TestRecordInvalidation_JournalWriteFailure(t *testing.T) {
	// Skip on Windows where chmod doesn't reliably make directories read-only
	if runtime.GOOS == "windows" {
		t.Skip("chmod-based write permission test not reliable on Windows")
	}

	t.Parallel()
	vaultPath := t.TempDir()

	// Make .raven directory read-only to force journal write failure
	ravenDir := filepath.Join(vaultPath, ".raven")
	if err := os.MkdirAll(ravenDir, 0o755); err != nil {
		t.Fatalf("mkdir .raven: %v", err)
	}
	if err := os.Chmod(ravenDir, 0o555); err != nil {
		t.Fatalf("chmod .raven: %v", err)
	}
	defer os.Chmod(ravenDir, 0o755) // Restore for cleanup

	before := schema.New()
	after := schema.New()
	after.Types["meeting"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{},
	}

	_, _, err := RecordInvalidation(vaultPath, before, after)
	if err == nil {
		t.Error("expected error when journal write fails")
	}
}
