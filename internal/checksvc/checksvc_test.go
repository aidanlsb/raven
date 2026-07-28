package checksvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	vaultpkg "github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func runCheckTest(t *testing.T, vaultPath string, cfg *config.VaultConfig, sch *schema.Schema, opts Options) (*RunResult, error) {
	t.Helper()
	rt := &vaultruntime.Runtime{VaultPath: vaultPath, VaultCfg: cfg, Schema: sch}
	defer rt.Close()
	return Run(rt, opts)
}

func TestRun_FiltersParseErrorsBeforeCounting(t *testing.T) {
	t.Parallel()

	for _, tt := range []struct {
		name string
		opts Options
	}{
		{
			name: "issues filter excludes parse error",
			opts: Options{Issues: "missing_reference", ErrorsOnly: true},
		},
		{
			name: "exclude filter drops parse error",
			opts: Options{Exclude: "parse_error", ErrorsOnly: true},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			vault := testutil.NewTestVault(t).
				WithSchema(testutil.PersonProjectSchema()).
				WithFile("broken.md", "---\ntype: person\nname: [\n---\nbody\n").
				Build()

			sch, err := schema.Load(vault.Path)
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}

			result, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, tt.opts)
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			if got := result.ErrorCount; got != 0 {
				t.Fatalf("error count = %d, want 0", got)
			}
			if len(result.Issues) != 0 {
				t.Fatalf("issues = %v, want none", result.Issues)
			}

			jsonResult := BuildJSON(vault.Path, result)
			if got := jsonResult.ErrorCount; got != 0 {
				t.Fatalf("json error_count = %d, want 0", got)
			}
			if len(jsonResult.Issues) != 0 {
				t.Fatalf("json issues = %v, want none", jsonResult.Issues)
			}
		})
	}
}

func TestBuildJSON_MissingReferenceSummarySuggestsCreateMissing(t *testing.T) {
	t.Parallel()

	result := &RunResult{
		Issues: []check.Issue{
			{
				Type:       check.IssueMissingReference,
				Level:      check.LevelError,
				FilePath:   "project/roadmap.md",
				Line:       4,
				Message:    "Reference [[meeting/all-hands]] not found",
				Value:      "meeting/all-hands",
				FixCommand: `rvn new meeting "meeting/all-hands"`,
				FixHint:    "Create the missing meeting",
			},
		},
		ErrorCount: 1,
	}

	jsonResult := BuildJSON("/vault", result)
	if len(jsonResult.Summary) != 1 {
		t.Fatalf("summary = %#v, want one item", jsonResult.Summary)
	}
	summary := jsonResult.Summary[0]
	if summary.IssueType != string(check.IssueMissingReference) {
		t.Fatalf("issue_type = %q, want missing_reference", summary.IssueType)
	}
	if summary.FixCommand != "rvn check create-missing --json" {
		t.Fatalf("fix_command = %q, want create-missing preview command", summary.FixCommand)
	}
	if !strings.Contains(summary.FixHint, "--confirm") {
		t.Fatalf("fix_hint = %q, want confirm guidance", summary.FixHint)
	}
}

func TestRun_ReportsMissingAssetReference(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("note.md", "See [paper](assets/pdfs/missing.pdf).\n").
		Build()
	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	result, err := runCheckTest(t, vault.Path, config.DefaultVaultConfig(), sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasIssue(result.Issues, check.IssueMissingAsset) {
		t.Fatalf("issues = %#v, want missing_asset", result.Issues)
	}
}

func TestRun_ReportsBrokenFileLinksAndSkipsURLs(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/source.md", strings.Join([]string{
			"[existing](../files/existing.txt)",
			"[missing](../files/missing.txt)",
			"[site](https://example.com/files/missing.txt)",
			"",
		}, "\n")).
		WithFile("files/existing.txt", "present\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	content := vault.ReadFile("notes/source.md")
	doc, err := parser.ParseDocument(content, filepath.Join(vault.Path, "notes", "source.md"), vault.Path)
	if err != nil {
		t.Fatalf("parse source: %v", err)
	}
	db, err := index.Open(vault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := db.IndexDocument(doc, sch); err != nil {
		_ = db.Close()
		t.Fatalf("index source: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	result, err := runCheckTest(t, vault.Path, config.DefaultVaultConfig(), sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	var broken []check.Issue
	for _, issue := range result.Issues {
		if issue.Type == check.IssueBrokenFileLink {
			broken = append(broken, issue)
		}
	}
	if len(broken) != 1 {
		t.Fatalf("broken_file_link issues = %#v, want one missing file only", broken)
	}
	if broken[0].Value != "../files/missing.txt" || broken[0].Line != 2 {
		t.Fatalf("broken issue = %#v, want missing target on line 2", broken[0])
	}
	if !strings.Contains(broken[0].FixHint, "Restore") {
		t.Fatalf("fix hint = %q, want restore/update guidance", broken[0].FixHint)
	}
}

func TestRun_ReportsOrphanedAsset(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	assetRel := "assets/random/paper.pdf"
	if err := os.MkdirAll(filepath.Join(vaultPath, "assets", "random"), 0o755); err != nil {
		t.Fatalf("mkdir asset dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, assetRel), []byte("%PDF test\n"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 1\ntypes: {}\ntraits: {}\n"), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	sch, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	cfg := config.DefaultVaultConfig()
	info, err := os.Stat(filepath.Join(vaultPath, assetRel))
	if err != nil {
		t.Fatalf("stat asset: %v", err)
	}
	db, err := index.Open(vaultPath)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := db.IndexAsset(vaultpkg.BuildAsset(assetRel, info, cfg)); err != nil {
		t.Fatalf("index asset: %v", err)
	}
	db.Close()

	result, err := runCheckTest(t, vaultPath, cfg, sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasIssue(result.Issues, check.IssueOrphanedAsset) {
		t.Fatalf("issues = %#v, want orphaned_asset", result.Issues)
	}
}

func TestRun_IgnoresExcludedMarkdownAndAssetIssues(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("directories:\n  type: type/\n  page: page/\nexclude:\n  - AGENTS.md\n  - .cursor/\n  - assets/generated/**\n").
		WithFile("page/keep.md", "# Keep\n").
		WithFile("AGENTS.md", "---\ntype: person\nname: [\n---\n").
		WithFile(".cursor/plans/work.plan.md", "---\ntype: person\nname: [\n---\n").
		Build()

	assetRel := "assets/generated/drop.pdf"
	assetFull := filepath.Join(vault.Path, assetRel)
	if err := os.MkdirAll(filepath.Dir(assetFull), 0o755); err != nil {
		t.Fatalf("mkdir asset: %v", err)
	}
	if err := os.WriteFile(assetFull, []byte("%PDF test\n"), 0o644); err != nil {
		t.Fatalf("write asset: %v", err)
	}
	cfg, err := config.LoadVaultConfig(vault.Path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	info, err := os.Stat(assetFull)
	if err != nil {
		t.Fatalf("stat asset: %v", err)
	}
	db, err := index.Open(vault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	if err := db.IndexAsset(vaultpkg.BuildAsset(assetRel, info, cfg)); err != nil {
		t.Fatalf("index asset: %v", err)
	}
	db.Close()

	result, err := runCheckTest(t, vault.Path, cfg, sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.FileCount != 1 {
		t.Fatalf("file count = %d, want only managed keep.md", result.FileCount)
	}
	if len(result.Issues) != 0 {
		t.Fatalf("issues = %#v, want none", result.Issues)
	}
}

func hasIssue(issues []check.Issue, issueType check.IssueType) bool {
	for _, issue := range issues {
		if issue.Type == issueType {
			return true
		}
	}
	return false
}

func TestRun_ReportsCheckIncompleteWhenIndexUnavailable(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/ok.md", "---\ntype: person\nname: Ok\n---\nbody\n").
		Build()

	ravenDir := filepath.Join(vault.Path, ".raven")
	if err := os.RemoveAll(ravenDir); err != nil {
		t.Fatalf("remove .raven: %v", err)
	}
	if err := os.WriteFile(ravenDir, []byte("not a directory"), 0o644); err != nil {
		t.Fatalf("write .raven file: %v", err)
	}

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	result, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasIssue(result.Issues, check.IssueCheckIncomplete) {
		t.Fatalf("issues = %#v, want check_incomplete", result.Issues)
	}
	found := false
	for _, issue := range result.Issues {
		if issue.Type == check.IssueCheckIncomplete && issue.Value == "index" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("issues = %#v, want check_incomplete for index subsystem", result.Issues)
	}
}
