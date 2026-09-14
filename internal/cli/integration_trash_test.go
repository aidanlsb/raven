//go:build integration

package cli_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_TrashListRestoreAndIndexHealing(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("projects/roadmap.md", "---\ntype: project\ntitle: Roadmap\nowner: \"[[people/freya]]\"\n---\n").
		Build()
	v.RunCLI("reindex").MustSucceed(t)
	v.AssertBacklinks("people/freya", 1)

	v.RunCLI("delete", "people/freya").MustSucceed(t)
	v.AssertFileNotExists("people/freya.md")
	v.AssertFileExists(".trash/people/freya.md")

	listed := v.RunCLI("trash", "list").MustSucceed(t)
	items := listed.DataList("items")
	if len(items) != 1 {
		t.Fatalf("trash items = %#v, want one", items)
	}
	item, ok := items[0].(map[string]interface{})
	if !ok {
		t.Fatalf("trash item = %#v, want object", items[0])
	}
	if item["reference"] != "people/freya" ||
		item["trash_path"] != ".trash/people/freya.md" ||
		item["restore_path"] != "people/freya.md" ||
		item["kind"] != "markdown" {
		t.Fatalf("trash item = %#v", item)
	}

	preview := v.RunCLI("restore", "people/freya").MustSucceed(t)
	if preview.Data["preview"] != true || preview.DataString("restore_path") != "people/freya.md" {
		t.Fatalf("restore preview = %s", preview.RawJSON)
	}
	v.AssertFileNotExists("people/freya.md")
	v.AssertFileExists(".trash/people/freya.md")

	applied := v.RunCLI("restore", "people/freya", "--confirm").MustSucceed(t)
	if applied.DataString("restored") != "people/freya" {
		t.Fatalf("restored = %q, want people/freya", applied.DataString("restored"))
	}
	v.AssertFileExists("people/freya.md")
	v.AssertFileNotExists(".trash/people/freya.md")

	query := v.RunCLI("query", "type:person").MustSucceed(t)
	query.AssertResultCount(t, "items", 1)
	v.AssertBacklinks("people/freya", 1)
}

func TestIntegration_RestoreCollisionAndMissingEntry(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		Build()
	v.RunCLI("reindex").MustSucceed(t)
	v.RunCLI("delete", "people/freya").MustSucceed(t)
	v.WriteFile("people/freya.md", "---\ntype: person\nname: Current Freya\n---\n")

	v.RunCLI("restore", "people/freya", "--confirm").MustFail(t, "FILE_EXISTS")
	v.AssertFileExists(".trash/people/freya.md")
	v.AssertFileContains("people/freya.md", "Current Freya")

	v.RunCLI("restore", "people/missing", "--confirm").MustFail(t, "FILE_NOT_FOUND")
}

func TestIntegration_TrashUsesConfiguredDirectoryForFiles(t *testing.T) {
	t.Parallel()

	const filePath = "files/archive/report.pdf"
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("deletion:\n  behavior: trash\n  trash_dir: archive/trash\n").
		WithFile(filePath, "pdf").
		Build()

	v.RunCLI("delete", filePath).MustSucceed(t)
	v.AssertFileNotExists(filePath)
	v.AssertFileExists("archive/trash/" + filePath)

	listed := v.RunCLI("trash", "list", "--kind", "file").MustSucceed(t)
	items := listed.DataList("items")
	if len(items) != 1 {
		t.Fatalf("trash items = %#v, want one", items)
	}
	item := items[0].(map[string]interface{})
	if item["reference"] != filePath || item["trash_path"] != "archive/trash/"+filePath {
		t.Fatalf("trash item = %#v", item)
	}

	v.RunCLI("restore", "archive/trash/"+filePath, "--confirm").MustSucceed(t)
	v.AssertFileExists(filePath)
	v.AssertFileNotExists("archive/trash/" + filePath)
}

func TestIntegration_TrashRejectsUnsafeConfiguredDirectory(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithRavenYAML("deletion:\n  behavior: trash\n  trash_dir: ../outside\n").
		WithFile("notes/keep.md", "# Keep\n").
		Build()

	v.RunCLI("delete", "notes/keep").MustFail(t, "CONFIG_INVALID")
	v.AssertFileExists("notes/keep.md")
	v.RunCLI("trash", "list").MustFail(t, "CONFIG_INVALID")
}

func TestIntegration_TrashEmptyPreviewConfirmAndLiveBoundary(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/freya.md", "---\ntype: person\nname: Freya\n---\n").
		WithFile("people/loki.md", "---\ntype: person\nname: Loki\n---\n").
		Build()

	v.RunCLI("delete", "people/freya").MustSucceed(t)
	v.AssertFileNotExists("people/freya.md")
	v.AssertFileExists(".trash/people/freya.md")
	v.AssertFileExists("people/loki.md")

	preview := v.RunCLI("trash", "empty").MustSucceed(t)
	if preview.Data["preview"] != true {
		t.Fatalf("empty preview = %s", preview.RawJSON)
	}
	items := preview.DataList("items")
	if len(items) != 1 {
		t.Fatalf("empty preview items = %#v, want one", items)
	}
	item := items[0].(map[string]interface{})
	if item["trash_path"] != ".trash/people/freya.md" {
		t.Fatalf("empty preview item = %#v", item)
	}
	if preview.Data["total"] != float64(1) {
		t.Fatalf("empty preview total = %#v, want 1", preview.Data["total"])
	}
	v.AssertFileExists(".trash/people/freya.md")
	v.AssertFileExists("people/loki.md")

	applied := v.RunCLI("trash", "empty", "--confirm").MustSucceed(t)
	if applied.Data["preview"] != false {
		t.Fatalf("empty apply preview flag = %s", applied.RawJSON)
	}
	if applied.Data["removed"] != float64(1) || applied.Data["total"] != float64(1) {
		t.Fatalf("empty apply counts = %s", applied.RawJSON)
	}
	v.AssertFileNotExists(".trash/people/freya.md")
	v.AssertFileExists("people/loki.md")
}

func TestIntegration_TrashEmptyOlderThan(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/old.md", "---\ntype: person\nname: Old\n---\n").
		WithFile("people/recent.md", "---\ntype: person\nname: Recent\n---\n").
		WithFile("people/live.md", "---\ntype: person\nname: Live\n---\n").
		Build()

	v.RunCLI("delete", "people/old").MustSucceed(t)
	v.RunCLI("delete", "people/recent").MustSucceed(t)

	now := time.Now()
	if err := os.Chtimes(filepath.Join(v.Path, ".trash/people/old.md"), now.Add(-10*24*time.Hour), now.Add(-10*24*time.Hour)); err != nil {
		t.Fatalf("chtimes old: %v", err)
	}
	if err := os.Chtimes(filepath.Join(v.Path, ".trash/people/recent.md"), now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("chtimes recent: %v", err)
	}

	preview := v.RunCLI("trash", "empty", "--older-than", "7d").MustSucceed(t)
	if preview.Data["preview"] != true || preview.Data["older_than"] != "7d" {
		t.Fatalf("older-than preview = %s", preview.RawJSON)
	}
	items := preview.DataList("items")
	if len(items) != 1 {
		t.Fatalf("older-than preview items = %#v, want one", items)
	}
	item := items[0].(map[string]interface{})
	if item["trash_path"] != ".trash/people/old.md" {
		t.Fatalf("older-than preview item = %#v", item)
	}
	v.AssertFileExists(".trash/people/old.md")
	v.AssertFileExists(".trash/people/recent.md")
	v.AssertFileExists("people/live.md")

	applied := v.RunCLI("trash", "empty", "--older-than", "7d", "--confirm").MustSucceed(t)
	if applied.Data["removed"] != float64(1) {
		t.Fatalf("older-than apply = %s", applied.RawJSON)
	}
	v.AssertFileNotExists(".trash/people/old.md")
	v.AssertFileExists(".trash/people/recent.md")
	v.AssertFileExists("people/live.md")
}

func TestIntegration_TrashEmptyRejectsInvalidOlderThan(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	v.RunCLI("trash", "empty", "--older-than", "last week").MustFail(t, "INVALID_INPUT")
}
