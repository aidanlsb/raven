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
  source:
    type: url
  source_list:
    type: url[]
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
		if got := schema.Traits["source"].Type; got != FieldTypeURL {
			t.Fatalf("expected trait type %q for source, got %q", FieldTypeURL, got)
		}
		if got := schema.Traits["source_list"].Type; got != FieldTypeURLArray {
			t.Fatalf("expected trait type %q for source_list, got %q", FieldTypeURLArray, got)
		}
	})

	t.Run("load optional type and field descriptions", func(t *testing.T) {
		tmpDir := t.TempDir()

		schemaContent := `
types:
  person:
    description: People and contacts
    fields:
      name:
        type: string
        description: Full display name
`
		err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0644)
		if err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		schema, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		person := schema.Types["person"]
		if person == nil {
			t.Fatal("expected 'person' type to exist")
		}
		if person.Description != "People and contacts" {
			t.Fatalf("expected type description %q, got %q", "People and contacts", person.Description)
		}

		nameField := person.Fields["name"]
		if nameField == nil {
			t.Fatal("expected 'name' field to exist")
		}
		if nameField.Description != "Full display name" {
			t.Fatalf("expected field description %q, got %q", "Full display name", nameField.Description)
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
      website:
        type: url
      links:
        type: url[]
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
		websiteField := typeDef.Fields["website"]
		if websiteField == nil {
			t.Fatal("expected 'website' field to exist")
		}
		if websiteField.Type != FieldTypeURL {
			t.Fatalf("expected website field type %q, got %q", FieldTypeURL, websiteField.Type)
		}
		linksField := typeDef.Fields["links"]
		if linksField == nil {
			t.Fatal("expected 'links' field to exist")
		}
		if linksField.Type != FieldTypeURLArray {
			t.Fatalf("expected links field type %q, got %q", FieldTypeURLArray, linksField.Type)
		}
	})

	t.Run("rejects inline template definitions", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaContent := `
types:
  meeting:
    template: |
      # {{title}}
      
      ## Notes
`
		if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		if _, err := Load(tmpDir); err == nil {
			t.Fatal("expected error for inline template definition, got nil")
		}
	})
}
