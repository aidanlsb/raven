package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewAutoFillsTitleFieldFromPositionalTitle(t *testing.T) {
	vaultPath := t.TempDir()

	// Minimal schema: `book` has a required `title` field.
	schemaYAML := strings.TrimSpace(`
version: 2
types:
  book:
    default_path: books/
    fields:
      title:
        type: string
        required: true
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	// Isolate global state used by the CLI package.
	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = nil // simulate MCP/agent that didn't provide --field title=...

	if err := newCmd.RunE(newCmd, []string{"book", "My Book"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "books", "my-book.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	got := string(b)

	if !strings.Contains(got, "type: book") {
		t.Fatalf("expected type frontmatter, got:\n%s", got)
	}
	if !strings.Contains(got, "title: My Book") {
		t.Fatalf("expected auto-filled title field, got:\n%s", got)
	}
}

func TestNewDoesNotOverrideExplicitTitleField(t *testing.T) {
	vaultPath := t.TempDir()

	schemaYAML := strings.TrimSpace(`
version: 2
types:
  book:
    default_path: books/
    fields:
      title:
        type: string
        required: true
`) + "\n"

	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte(schemaYAML), 0644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	prevVault := resolvedVaultPath
	prevJSON := jsonOutput
	prevFields := newFieldFlags
	t.Cleanup(func() {
		resolvedVaultPath = prevVault
		jsonOutput = prevJSON
		newFieldFlags = prevFields
	})

	resolvedVaultPath = vaultPath
	jsonOutput = true
	newFieldFlags = []string{"title=Override Title"}

	if err := newCmd.RunE(newCmd, []string{"book", "My Book 2"}); err != nil {
		t.Fatalf("newCmd.RunE: %v", err)
	}

	created := filepath.Join(vaultPath, "books", "my-book-2.md")
	b, err := os.ReadFile(created)
	if err != nil {
		t.Fatalf("read created file: %v", err)
	}
	got := string(b)

	if strings.Contains(got, "title: My Book 2") {
		t.Fatalf("did not expect positional title to override explicit field, got:\n%s", got)
	}
	if !strings.Contains(got, "title: Override Title") {
		t.Fatalf("expected explicit title field to be preserved, got:\n%s", got)
	}
}

