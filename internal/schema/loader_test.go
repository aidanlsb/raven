package schema

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	t.Run("default schema when no file", func(t *testing.T) {
		tmpDir := t.TempDir()

		schema, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := schema.Types["page"]; !ok {
			t.Error("expected 'page' type to exist")
		}

		if _, ok := schema.Types["section"]; !ok {
			t.Error("expected 'section' type to exist")
		}
	})

	t.Run("load custom schema", func(t *testing.T) {
		tmpDir := t.TempDir()

		schemaContent := `
types:
  person:
    fields:
      name:
        type: string
        required: true
traits:
  task:
    fields:
      due:
        type: date
`
		err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0644)
		if err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		schema, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := schema.Types["person"]; !ok {
			t.Error("expected 'person' type to exist")
		}

		// Fallback types still added
		if _, ok := schema.Types["page"]; !ok {
			t.Error("expected 'page' type to exist")
		}

		if _, ok := schema.Traits["task"]; !ok {
			t.Error("expected 'task' trait to exist")
		}
	})

	t.Run("normalizes reference field types", func(t *testing.T) {
		tmpDir := t.TempDir()

		schemaContent := `
types:
  person:
    fields:
      company:
        type: reference
      teammates:
        type: reference[]
`
		err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0644)
		if err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		schema, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		typeDef := schema.Types["person"]
		if typeDef == nil {
			t.Fatal("expected 'person' type to exist")
		}
		companyField := typeDef.Fields["company"]
		if companyField == nil {
			t.Fatal("expected 'company' field to exist")
		}
		if companyField.Type != FieldTypeRef {
			t.Fatalf("expected company field type %q, got %q", FieldTypeRef, companyField.Type)
		}
		teammatesField := typeDef.Fields["teammates"]
		if teammatesField == nil {
			t.Fatal("expected 'teammates' field to exist")
		}
		if teammatesField.Type != FieldTypeRefArray {
			t.Fatalf("expected teammates field type %q, got %q", FieldTypeRefArray, teammatesField.Type)
		}
	})
}
