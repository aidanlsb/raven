package checksvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vault"
)

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

			result, err := Run(vault.Path, &config.VaultConfig{}, sch, tt.opts)
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

func TestCreateMissingRefsNonInteractive_ReportsFailures(t *testing.T) {
	t.Parallel()

	result := CreateMissingRefsNonInteractive(
		t.TempDir(),
		&schema.Schema{
			Types: map[string]*schema.TypeDefinition{
				"meeting": {DefaultPath: "meeting/"},
			},
		},
		[]*check.MissingRef{
			{TargetPath: "all-hands", InferredType: "meeting"},
		},
		"",
		"",
		"",
		[]string{"meeting/"},
	)

	if result.Created != 0 {
		t.Fatalf("created = %d, want 0", result.Created)
	}
	if len(result.Failures) != 1 {
		t.Fatalf("failures = %v, want 1 failure", result.Failures)
	}
	if !strings.Contains(result.Failures[0].Error, "protected") {
		t.Fatalf("unexpected failure error: %v", result.Failures[0])
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

	result, err := Run(vault.Path, config.DefaultVaultConfig(), sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasIssue(result.Issues, check.IssueMissingAsset) {
		t.Fatalf("issues = %#v, want missing_asset", result.Issues)
	}
}

func TestRun_ReportsOrphanedAndNonCanonicalAsset(t *testing.T) {
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
	if err := db.IndexAsset(vault.BuildAsset(assetRel, info, cfg)); err != nil {
		t.Fatalf("index asset: %v", err)
	}
	db.Close()

	result, err := Run(vaultPath, cfg, sch, Options{})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !hasIssue(result.Issues, check.IssueOrphanedAsset) {
		t.Fatalf("issues = %#v, want orphaned_asset", result.Issues)
	}
	if !hasIssue(result.Issues, check.IssueNonCanonicalAsset) {
		t.Fatalf("issues = %#v, want non_canonical_asset", result.Issues)
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
