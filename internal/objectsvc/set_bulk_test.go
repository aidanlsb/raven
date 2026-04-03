package objectsvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
)

func TestApplySetBulkTypedUpdatesPreservesStringType(t *testing.T) {
	t.Parallel()
	vaultPath := t.TempDir()
	writeTestSchema(t, vaultPath, `
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string
traits: {}
`)
	sch := loadTestSchema(t, vaultPath)

	people := map[string]string{
		"one": "One",
		"two": "Two",
	}
	for slug, title := range people {
		filePath := filepath.Join(vaultPath, "people", slug+".md")
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", slug, err)
		}
		content := "---\ntype: person\nname: " + title + "\n---\n"
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatalf("seed %s: %v", slug, err)
		}
	}

	summary, err := ApplySetBulk(SetBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: &config.VaultConfig{},
		Schema:      sch,
		ObjectIDs:   []string{"people/one", "people/two"},
		TypedUpdates: map[string]schema.FieldValue{
			"email": schema.String("true"),
		},
	}, nil)
	if err != nil {
		t.Fatalf("ApplySetBulk: %v", err)
	}
	if summary.Modified != 2 {
		t.Fatalf("modified = %d, want 2", summary.Modified)
	}
	if summary.Errors != 0 {
		t.Fatalf("errors = %d, want 0", summary.Errors)
	}

	for _, person := range []string{"one", "two"} {
		updated, err := os.ReadFile(filepath.Join(vaultPath, "people", person+".md"))
		if err != nil {
			t.Fatalf("read %s: %v", person, err)
		}
		if !strings.Contains(string(updated), `email: "true"`) {
			t.Fatalf("expected %s email to remain a string, got:\n%s", person, string(updated))
		}
	}
}
