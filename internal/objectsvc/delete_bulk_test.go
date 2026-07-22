package objectsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestDeleteBulkAssetPreviewAndApply(t *testing.T) {
	t.Parallel()

	const assetID = "assets/images/example/chart.png"
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		WithFile(assetID, "png").
		WithFile("notes/reference.md", "![Chart]("+assetID+")\n").
		Build()
	sch := loadTestSchema(t, v.Path)
	indexVaultFiles(t, v.Path, sch, "notes/reference.md")
	indexVaultAssets(t, v.Path, assetID)
	resolveVaultRefs(t, v.Path, sch)

	req := DeleteBulkRequest{
		VaultPath:   v.Path,
		VaultConfig: config.DefaultVaultConfig(),
		ObjectIDs:   []string{assetID},
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
	if len(preview.Items) != 1 || preview.Items[0].ID != assetID {
		t.Fatalf("Items = %#v, want %q", preview.Items, assetID)
	}
	if preview.Items[0].Details != "⚠ referenced by 1 objects" {
		t.Fatalf("Details = %q, want backlink warning", preview.Items[0].Details)
	}
	v.AssertFileExists(assetID)

	summary, err := ApplyDeleteBulk(req)
	if err != nil {
		t.Fatalf("ApplyDeleteBulk() error = %v", err)
	}
	if summary.Deleted != 1 || summary.Skipped != 0 || summary.Errors != 0 {
		t.Fatalf("summary = %#v, want one deleted asset", summary)
	}
	if len(summary.WarningMessages) != 0 {
		t.Fatalf("WarningMessages = %#v, want none", summary.WarningMessages)
	}
	v.AssertFileNotExists(assetID)
	v.AssertFileExists(".trash/" + assetID)

	db, err := index.Open(v.Path)
	if err != nil {
		t.Fatalf("open index: %v", err)
	}
	defer db.Close()
	assets, err := db.QueryAssets()
	if err != nil {
		t.Fatalf("QueryAssets() error = %v", err)
	}
	if len(assets) != 0 {
		t.Fatalf("QueryAssets() = %#v, want no assets after bulk delete", assets)
	}
}
