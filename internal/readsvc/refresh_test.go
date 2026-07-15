package readsvc

import (
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestSmartReindexReportsParseFailures(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/ok.md", "---\ntype: person\nname: Ok\n---\nbody\n").
		WithFile("people/broken.md", "---\ntype: person\nname: [\n---\nbody\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	db, err := index.Open(vault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rt := &Runtime{
		VaultPath: vault.Path,
		VaultCfg:  &config.VaultConfig{},
		Schema:    sch,
		DB:        db,
	}

	report, err := SmartReindex(rt)
	if err != nil {
		t.Fatalf("SmartReindex: %v", err)
	}
	if report.Indexed < 1 {
		t.Fatalf("indexed = %d, want at least 1", report.Indexed)
	}
	if len(report.Failures) != 1 {
		t.Fatalf("failures = %#v, want 1 parse failure", report.Failures)
	}
	if report.Failures[0].Stage != "parse" {
		t.Fatalf("stage = %q, want parse", report.Failures[0].Stage)
	}
	if report.Failures[0].Path != filepath.ToSlash("people/broken.md") {
		t.Fatalf("path = %q, want people/broken.md", report.Failures[0].Path)
	}
}
