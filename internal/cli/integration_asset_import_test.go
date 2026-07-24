//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_AssetImportCopyIndexesAsset(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	source := writeExternalAsset(t, "paper.pdf", "%PDF-1.7\nasset import\n")

	result := v.RunCLI("asset", "import", source, "assets/pdfs/")
	result.MustSucceed(t)
	if got := result.DataString("id"); got != "assets/pdfs/paper.pdf" {
		t.Fatalf("id = %q, want assets/pdfs/paper.pdf; raw: %s", got, result.RawJSON)
	}
	if got := result.DataString("path"); got != "assets/pdfs/paper.pdf" {
		t.Fatalf("path = %q, want assets/pdfs/paper.pdf; raw: %s", got, result.RawJSON)
	}
	if got := result.DataString("media_type"); got != "application/pdf" {
		t.Fatalf("media_type = %q, want application/pdf; raw: %s", got, result.RawJSON)
	}
	if got := result.DataString("mode"); got != "copy" {
		t.Fatalf("mode = %q, want copy; raw: %s", got, result.RawJSON)
	}
	if got, ok := result.Data["size_bytes"].(float64); !ok || int(got) != len("%PDF-1.7\nasset import\n") {
		t.Fatalf("size_bytes = %#v; raw: %s", result.Data["size_bytes"], result.RawJSON)
	}
	items := result.DataList("items")
	if len(items) != 1 {
		t.Fatalf("items = %#v, want one imported asset", items)
	}

	v.AssertFileExists("assets/pdfs/paper.pdf")
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("copy removed source: %v", err)
	}

	query := v.RunCLI("query", "asset .extension==pdf")
	query.MustSucceed(t)
	query.AssertResultCount(t, "items", 1)
	item := query.DataList("items")[0].(map[string]interface{})
	if got := item["id"]; got != "assets/pdfs/paper.pdf" {
		t.Fatalf("indexed asset id = %#v", got)
	}
}

func TestIntegration_AssetImportMoveAndDryRun(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	previewSource := writeExternalAsset(t, "preview.csv", "a,b\n")
	preview := v.RunCLI("asset", "import", previewSource, "assets/data/", "--move", "--dry-run")
	preview.MustSucceed(t)
	if value, ok := preview.Data["preview"].(bool); !ok || !value {
		t.Fatalf("preview = %#v; raw: %s", preview.Data["preview"], preview.RawJSON)
	}
	v.AssertFileNotExists("assets/data/preview.csv")
	if _, err := os.Stat(previewSource); err != nil {
		t.Fatalf("dry run removed source: %v", err)
	}

	moveSource := writeExternalAsset(t, "move.csv", "x,y\n1,2\n")
	moved := v.RunCLI("asset", "import", moveSource, "assets/data/", "--move")
	moved.MustSucceed(t)
	if got := moved.DataString("mode"); got != "move" {
		t.Fatalf("mode = %q, want move", got)
	}
	if removed, ok := moved.Data["source_removed"].(bool); !ok || !removed {
		t.Fatalf("source_removed = %#v; raw: %s", moved.Data["source_removed"], moved.RawJSON)
	}
	if _, err := os.Stat(moveSource); !os.IsNotExist(err) {
		t.Fatalf("move source still exists: %v", err)
	}
	v.AssertFileExists("assets/data/move.csv")
}

func TestIntegration_AssetImportCollisionAndValidation(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile("assets/data/existing.csv", "old").
		Build()

	source := writeExternalAsset(t, "existing.csv", "new")
	collision := v.RunCLI("asset", "import", source, "assets/data/existing.csv")
	collision.MustFail(t, "FILE_EXISTS")
	if got := v.ReadFile("assets/data/existing.csv"); got != "old" {
		t.Fatalf("collision changed destination to %q", got)
	}

	forced := v.RunCLI("asset", "import", source, "assets/data/existing.csv", "--force")
	forced.MustSucceed(t)
	if got := v.ReadFile("assets/data/existing.csv"); got != "new" {
		t.Fatalf("forced import destination = %q, want new", got)
	}

	markdown := writeExternalAsset(t, "notes.md", "# Notes\n")
	v.RunCLI("asset", "import", markdown, "assets/notes.bin").MustFail(t, "INVALID_INPUT")
	v.RunCLI("asset", "import", source, "page/existing.csv").MustFail(t, "FILE_OUTSIDE_VAULT")
	v.RunCLI("asset", "import", source, "assets/data/no-extension").MustFail(t, "INVALID_INPUT")
}

func TestIntegration_AssetImportUsesCustomRootAndExistingDirectory(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("directories:\n  assets: files/\n").
		WithFile("files/uploads/.keep", "keep").
		Build()
	source := writeExternalAsset(t, "photo.png", "png")

	result := v.RunCLI("asset", "import", source, "files/uploads")
	result.MustSucceed(t)
	if got := result.DataString("path"); got != "files/uploads/photo.png" {
		t.Fatalf("path = %q, want files/uploads/photo.png", got)
	}
	v.AssertFileExists("files/uploads/photo.png")
}

func TestIntegration_AssetImportJournalGuardBlocksWrite(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()
	if err := os.WriteFile(filepath.Join(v.Path, ".raven"), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := writeExternalAsset(t, "blocked.pdf", "pdf")

	result := v.RunCLI("asset", "import", source, "assets/blocked.pdf", "--move")
	result.MustFail(t, "DATABASE_ERROR")
	v.AssertFileNotExists("assets/blocked.pdf")
	if _, err := os.Stat(source); err != nil {
		t.Fatalf("guard failure removed source: %v", err)
	}
}

func writeExternalAsset(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
