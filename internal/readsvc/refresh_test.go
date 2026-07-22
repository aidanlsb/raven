package readsvc

import (
	"os"
	"path/filepath"
	"strings"
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

func TestSmartReindexSkipsParsingUnchangedMarkdown(t *testing.T) {
	t.Parallel()

	testVault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\nbody\n").
		Build()

	sch, err := schema.Load(testVault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	db, err := index.Open(testVault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	rt := &Runtime{
		VaultPath: testVault.Path,
		VaultCfg:  &config.VaultConfig{},
		Schema:    sch,
		DB:        db,
	}
	if report, err := SmartReindex(rt); err != nil {
		t.Fatalf("initial SmartReindex: %v", err)
	} else if report.Indexed != 1 {
		t.Fatalf("initial indexed = %d, want 1", report.Indexed)
	}

	filePath := filepath.Join(testVault.Path, "people", "freya.md")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatalf("stat indexed markdown: %v", err)
	}
	if err := os.WriteFile(filePath, []byte("---\ntype: [invalid yaml\n---\n"), 0o644); err != nil {
		t.Fatalf("replace indexed markdown: %v", err)
	}
	if err := os.Chtimes(filePath, info.ModTime(), info.ModTime()); err != nil {
		t.Fatalf("restore indexed markdown mtime: %v", err)
	}

	report, err := SmartReindex(rt)
	if err != nil {
		t.Fatalf("SmartReindex: %v", err)
	}
	if report.Indexed != 0 {
		t.Fatalf("indexed = %d, want 0", report.Indexed)
	}
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %#v, want none because unchanged file must not be parsed", report.Failures)
	}
}

func TestSmartReindexWarnsOnUnknownFrontmatter(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\nfavorite_color: blue\n---\nbody\n").
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
	if len(report.Failures) != 0 {
		t.Fatalf("failures = %#v, want none", report.Failures)
	}
	if len(report.Warnings) == 0 {
		t.Fatal("expected unknown frontmatter warning")
	}
	found := false
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, "favorite_color") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("warnings = %#v, want favorite_color mention", report.Warnings)
	}

	obj, err := db.GetObject("people/freya")
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if _, ok := obj.Fields["favorite_color"]; ok {
		t.Fatalf("expected favorite_color omitted from index, got %#v", obj.Fields)
	}
}
