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

	t.Run("loads schema templates and type template IDs", func(t *testing.T) {
		tmpDir := t.TempDir()

		schemaContent := `
templates:
  interview_technical:
    file: templates/interview/technical.md
types:
  interview:
    templates: [interview_technical]
    default_template: interview_technical
`
		if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		loaded, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if _, ok := loaded.Templates["interview_technical"]; !ok {
			t.Fatal("expected schema template interview_technical to exist")
		}
		typeDef := loaded.Types["interview"]
		if typeDef == nil {
			t.Fatal("expected interview type")
		}
		if len(typeDef.Templates) != 1 || typeDef.Templates[0] != "interview_technical" {
			t.Fatalf("expected interview.templates=[%q], got %v", "interview_technical", typeDef.Templates)
		}
		if got := typeDef.DefaultTemplate; got != "interview_technical" {
			t.Fatalf("expected default_template=%q, got %q", "interview_technical", got)
		}
	})

	t.Run("preserves date type template bindings", func(t *testing.T) {
		tmpDir := t.TempDir()

		schemaContent := `
templates:
  daily_default:
    file: templates/daily.md
types:
  date:
    templates: [daily_default]
    default_template: daily_default
`
		if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		loaded, err := Load(tmpDir)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		dateType := loaded.Types["date"]
		if dateType == nil {
			t.Fatal("expected built-in date type")
		}
		if len(dateType.Templates) != 1 || dateType.Templates[0] != "daily_default" {
			t.Fatalf("expected date.templates=[%q], got %v", "daily_default", dateType.Templates)
		}
		if dateType.DefaultTemplate != "daily_default" {
			t.Fatalf("expected date.default_template=%q, got %q", "daily_default", dateType.DefaultTemplate)
		}
	})

	t.Run("rejects unknown type template id", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaContent := `
types:
  interview:
    templates: [interview_technical]
`
		if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		if _, err := Load(tmpDir); err == nil {
			t.Fatal("expected error for unknown template ID, got nil")
		}
	})

	t.Run("rejects default_template missing template ID", func(t *testing.T) {
		tmpDir := t.TempDir()
		schemaContent := `
templates:
  interview_technical:
    file: templates/interview/technical.md
types:
  interview:
    templates: [interview_technical]
    default_template: interview_screen
`
		if err := os.WriteFile(filepath.Join(tmpDir, "schema.yaml"), []byte(schemaContent), 0o644); err != nil {
			t.Fatalf("failed to write schema: %v", err)
		}

		if _, err := Load(tmpDir); err == nil {
			t.Fatal("expected error for invalid default_template template ID, got nil")
		}
	})
}
