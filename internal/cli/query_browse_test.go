package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestBrowseItemsForObjectResultsUseNameFieldAndDetails(t *testing.T) {
	sch := schema.New()
	sch.Types["issue"] = &schema.TypeDefinition{
		NameField: "title",
		Fields: map[string]*schema.FieldDefinition{
			"title":   {Type: schema.FieldTypeString},
			"project": {Type: schema.FieldTypeRef},
			"status":  {Type: schema.FieldTypeString},
		},
	}

	items := browseItemsForObjectResults([]model.Object{
		{
			ID:        "issue/check-if-queries-return-the-fields-in-the-printed-table",
			Type:      "issue",
			FilePath:  "type/issue/check-if-queries-return-the-fields-in-the-printed-table.md",
			LineStart: 1,
			Fields: map[string]interface{}{
				"title":   "Check if queries return the fields in the printed table",
				"project": "project/raven",
				"status":  "open",
			},
		},
	}, sch)

	if len(items) != 1 {
		t.Fatalf("expected 1 browse item, got %d", len(items))
	}
	item := items[0]
	if item.Label != "Check if queries return the fields in the printed table" {
		t.Fatalf("label = %q, want title field", item.Label)
	}
	if !strings.Contains(item.Detail, "project=raven") || !strings.Contains(item.Detail, "status=open") {
		t.Fatalf("detail = %q, want field summary", item.Detail)
	}
	if strings.Contains(item.Detail, item.ID) {
		t.Fatalf("detail should not repeat object id, got %q", item.Detail)
	}
}

func TestQueryBrowsePreviewHandlesShortFiles(t *testing.T) {
	vaultPath := t.TempDir()
	prevVaultPath := resolvedVaultPath
	resolvedVaultPath = vaultPath
	t.Cleanup(func() {
		resolvedVaultPath = prevVaultPath
	})

	relPath := filepath.Join("type", "issue", "short.md")
	fullPath := filepath.Join(vaultPath, relPath)
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(fullPath, []byte("---\ntype: issue\nstatus: open\n---\n\n# Notes\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	preview := queryBrowsePreview(relPath, 1)
	for _, want := range []string{"type: issue", "status: open", "# Notes"} {
		if !strings.Contains(preview, want) {
			t.Fatalf("preview missing %q: %q", want, preview)
		}
	}
}
