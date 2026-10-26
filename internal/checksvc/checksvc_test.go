package checksvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
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
		})
	}
}

func TestRun_TypeFilterKeepsMatchingDocuments(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\nunknown: yes\n---\n").
		WithFile("projects/website.md", "---\ntype: project\ntitle: Website\nowner: people/ghost\n---\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	result, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, Options{TypeFilter: "person"})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	for _, issue := range result.Issues {
		if issue.FilePath == "projects/website.md" {
			t.Fatalf("type filter leaked project issue: %#v", issue)
		}
	}
	if !hasIssue(result.Issues, check.IssueUnknownFrontmatter) {
		t.Fatalf("issues = %#v, want unknown_frontmatter_key on the person file", result.Issues)
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
		WithFile("notes-old/source.md", "[outside scope](../files/also-missing.txt)\n").
		WithFile("files/existing.txt", "present\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	db, err := index.Open(vault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	for _, relPath := range []string{"notes/source.md", "notes-old/source.md"} {
		content := vault.ReadFile(relPath)
		doc, parseErr := parser.ParseDocument(content, filepath.Join(vault.Path, filepath.FromSlash(relPath)), vault.Path)
		if parseErr != nil {
			_ = db.Close()
			t.Fatalf("parse %s: %v", relPath, parseErr)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			_ = db.Close()
			t.Fatalf("index %s: %v", relPath, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close index: %v", err)
	}

	result, err := runCheckTest(t, vault.Path, config.DefaultVaultConfig(), sch, Options{PathArg: "notes"})
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

func TestRun_ReportsMarkdownLinksAndImagesToVaultNotes(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("notes/source.md", strings.Join([]string{
			"[note](../type/foo.md)",
			"![note image](../type/foo.md#details)",
			`[escaped note](../type/foo\#draft.md)`,
			"[file](../files/manual.pdf)",
			"[url](https://example.com/foo.md)",
			"[[type/foo]]",
			"",
		}, "\n")).
		WithFile("type/foo.md", "# Foo\n\n## Details\n").
		WithFile("type/foo#draft.md", "# Foo draft\n").
		WithFile("files/manual.pdf", "present\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
	result, err := runCheckTest(t, vault.Path, config.DefaultVaultConfig(), sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}

	var issues []check.Issue
	for _, issue := range result.Issues {
		if issue.Type == check.IssueMarkdownLinkToVaultNote {
			issues = append(issues, issue)
		}
	}
	if len(issues) != 3 {
		t.Fatalf("markdown_link_to_vault_note issues = %#v, want link, image, and escaped-filename link only", issues)
	}
	if issues[0].Value != "../type/foo.md" || issues[0].Line != 1 {
		t.Errorf("link issue = %#v, want target on line 1", issues[0])
	}
	if issues[1].Value != "../type/foo.md#details" || issues[1].Line != 2 {
		t.Errorf("image issue = %#v, want target on line 2", issues[1])
	}
	if issues[2].Value != `../type/foo\#draft.md` || issues[2].Line != 3 {
		t.Errorf("escaped-filename issue = %#v, want target on line 3", issues[2])
	}
	for _, issue := range issues {
		if issue.Level != check.LevelError {
			t.Errorf("issue level = %v, want error", issue.Level)
		}
		if !strings.Contains(issue.FixHint, "[[target]]") {
			t.Errorf("fix hint = %q, want wikilink guidance", issue.FixHint)
		}
	}
}

func TestRun_IgnoresExcludedMarkdown(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithRavenYAML("directories:\n  type: type/\n  page: page/\nexclude:\n  - AGENTS.md\n  - .cursor/\n").
		WithFile("page/keep.md", "# Keep\n").
		WithFile("AGENTS.md", "---\ntype: person\nname: [\n---\n").
		WithFile(".cursor/plans/work.plan.md", "---\ntype: person\nname: [\n---\n").
		Build()

	cfg, err := config.LoadVaultConfig(vault.Path)
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}
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

func TestRun_ReportsStaleIndexFromWalkMtimes(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/ada.md", "---\ntype: person\nname: Ada\n---\nbody\n").
		WithFile("people/grace.md", "---\ntype: person\nname: Grace\n---\nbody\n").
		Build()

	sch, err := schema.Load(vault.Path)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	indexVaultFiles(t, vault, sch, []string{"people/ada.md", "people/grace.md"})

	result, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, Options{})
	if err != nil {
		t.Fatalf("fresh check: %v", err)
	}
	if result.StaleWarningShown || hasIssue(result.Issues, check.IssueStaleIndex) {
		t.Fatalf("fresh index reported stale: %#v", result.Issues)
	}

	adaPath := filepath.Join(vault.Path, "people", "ada.md")
	later := time.Now().Add(2 * time.Second)
	if err := os.Chtimes(adaPath, later, later); err != nil {
		t.Fatalf("chtimes ada.md: %v", err)
	}

	stale, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, Options{})
	if err != nil {
		t.Fatalf("stale check: %v", err)
	}
	if !stale.StaleWarningShown || !hasIssue(stale.Issues, check.IssueStaleIndex) {
		t.Fatalf("touched file did not report stale_index: %#v", stale.Issues)
	}

	if err := os.Remove(filepath.Join(vault.Path, "people", "grace.md")); err != nil {
		t.Fatalf("remove grace.md: %v", err)
	}
	deleted, err := runCheckTest(t, vault.Path, &config.VaultConfig{}, sch, Options{})
	if err != nil {
		t.Fatalf("deleted-file check: %v", err)
	}
	if !deleted.StaleWarningShown || !hasIssue(deleted.Issues, check.IssueStaleIndex) {
		t.Fatalf("deleted indexed file did not report stale_index: %#v", deleted.Issues)
	}

	unindexed := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/new.md", "---\ntype: person\nname: New\n---\nbody\n").
		Build()
	unindexedSch, err := schema.Load(unindexed.Path)
	if err != nil {
		t.Fatalf("load unindexed schema: %v", err)
	}
	freshEmpty, err := runCheckTest(t, unindexed.Path, &config.VaultConfig{}, unindexedSch, Options{})
	if err != nil {
		t.Fatalf("unindexed check: %v", err)
	}
	if freshEmpty.StaleWarningShown || hasIssue(freshEmpty.Issues, check.IssueStaleIndex) {
		t.Fatalf("new unindexed file reported stale_index: %#v", freshEmpty.Issues)
	}
}

func indexVaultFiles(t *testing.T, vault *testutil.TestVault, sch *schema.Schema, relPaths []string) {
	t.Helper()
	db, err := index.Open(vault.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Fatalf("close index: %v", err)
		}
	}()
	for _, relPath := range relPaths {
		fullPath := filepath.Join(vault.Path, filepath.FromSlash(relPath))
		info, err := os.Stat(fullPath)
		if err != nil {
			t.Fatalf("stat %s: %v", relPath, err)
		}
		doc, parseErr := parser.ParseDocument(vault.ReadFile(relPath), fullPath, vault.Path)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", relPath, parseErr)
		}
		if err := db.IndexDocumentWithMtime(doc, sch, info.ModTime().Unix()); err != nil {
			t.Fatalf("index %s: %v", relPath, err)
		}
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
