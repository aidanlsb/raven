package objectsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestDeleteBulkFilePreviewAndApply(t *testing.T) {
	t.Parallel()

	const filePath = "files/images/example/chart.png"
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile(filePath, "png").
		Build()

	req := DeleteBulkRequest{
		VaultPath:   v.Path,
		VaultConfig: config.DefaultVaultConfig(),
		ObjectIDs:   []string{filePath},
		Behavior:    "trash",
		TrashDir:    ".trash",
	}
	preview, err := PreviewDeleteBulk(req)
	if err != nil {
		t.Fatalf("PreviewDeleteBulk() error = %v", err)
	}
	if len(preview.Skipped) != 0 {
		t.Fatalf("Skipped = %#v, want none", preview.Skipped)
	}
	if len(preview.Items) != 1 || preview.Items[0].ID != filePath {
		t.Fatalf("Items = %#v, want %q", preview.Items, filePath)
	}
	if preview.Items[0].Details != "" {
		t.Fatalf("Details = %q, want no Raven backlink warning", preview.Items[0].Details)
	}
	v.AssertFileExists(filePath)

	summary, err := ApplyDeleteBulk(req)
	if err != nil {
		t.Fatalf("ApplyDeleteBulk() error = %v", err)
	}
	if summary.Deleted != 1 || summary.Skipped != 0 || summary.Errors != 0 {
		t.Fatalf("summary = %#v, want one deleted file", summary)
	}
	v.AssertFileNotExists(filePath)
	v.AssertFileExists(".trash/" + filePath)
	if got := summary.ChangeSet.Deleted; len(got) != 1 || got[0] != filePath {
		t.Fatalf("ChangeSet.Deleted = %#v, want [%s]", got, filePath)
	}
}
