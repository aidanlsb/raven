package schemadoc

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestEditUsesTypedAndEditableViewsOfSameDocument(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	schemaPath := filepath.Join(vaultPath, "schema.yaml")
	input := `version: 1
types:
  person:
    traits: [reviewed]
    fields: {}
traits:
  reviewed:
    type: boolean
`
	if err := os.WriteFile(schemaPath, []byte(input), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	err := Edit(vaultPath, func(doc *Document) error {
		if _, ok := doc.Schema().Types["person"]; !ok {
			t.Fatal("typed schema did not contain person")
		}
		types := EnsureMap(doc.Root(), "types")
		person := EnsureMap(types, "person")
		person["description"] = "People and contacts"
		return nil
	})
	if err != nil {
		t.Fatalf("Edit: %v", err)
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load edited schema: %v", err)
	}
	if got := loaded.Types["person"].Description; got != "People and contacts" {
		t.Fatalf("description = %q, want %q", got, "People and contacts")
	}

	output, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read edited schema: %v", err)
	}
	var root map[string]interface{}
	if err := yaml.Unmarshal(output, &root); err != nil {
		t.Fatalf("decode edited schema: %v", err)
	}
	types := root["types"].(map[string]interface{})
	person := types["person"].(map[string]interface{})
	traits, ok := person["traits"].([]interface{})
	if !ok || len(traits) != 1 || traits[0] != "reviewed" {
		t.Fatalf("raw type traits = %#v, want [reviewed]", person["traits"])
	}
}

func TestEditRejectsInvalidStagedSchemaWithoutWriting(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	schemaPath := filepath.Join(vaultPath, "schema.yaml")
	input := []byte("version: 1\ntypes: {}\ntraits: {}\n")
	if err := os.WriteFile(schemaPath, input, 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	err := Edit(vaultPath, func(doc *Document) error {
		templates := EnsureMap(doc.Root(), "templates")
		templates["invalid"] = map[string]interface{}{"file": ""}
		return nil
	})
	if err == nil {
		t.Fatal("Edit succeeded with an invalid staged schema")
	}
	var docErr *Error
	if !errors.As(err, &docErr) {
		t.Fatalf("error = %T, want *schemadoc.Error", err)
	}
	if docErr.Operation != OperationValidate {
		t.Fatalf("operation = %q, want %q", docErr.Operation, OperationValidate)
	}

	output, readErr := os.ReadFile(schemaPath)
	if readErr != nil {
		t.Fatalf("read schema: %v", readErr)
	}
	if string(output) != string(input) {
		t.Fatalf("schema changed after rejected edit:\n%s", output)
	}
}

func TestEditNoChangeDoesNotRewriteDocument(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	schemaPath := filepath.Join(vaultPath, "schema.yaml")
	input := []byte("# retained because no write occurs\nversion: 1\ntypes: {}\ntraits: {}\n")
	if err := os.WriteFile(schemaPath, input, 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	if err := Edit(vaultPath, func(*Document) error { return ErrNoChange }); err != nil {
		t.Fatalf("Edit: %v", err)
	}

	output, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	if string(output) != string(input) {
		t.Fatalf("no-change edit rewrote schema:\n%s", output)
	}
}

func TestWriteValidatesBeforeReplacingSchema(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	schemaPath := filepath.Join(vaultPath, "schema.yaml")
	original := []byte("version: 1\ntypes: {}\ntraits: {}\n")
	if err := os.WriteFile(schemaPath, original, 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	invalid := []byte("version: 1\ntemplates:\n  broken:\n    file: \"\"\n")
	if err := Write(vaultPath, invalid); err == nil {
		t.Fatal("Write succeeded with invalid schema")
	}
	afterInvalid, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatalf("read schema after invalid write: %v", err)
	}
	if string(afterInvalid) != string(original) {
		t.Fatalf("invalid write replaced schema:\n%s", afterInvalid)
	}

	valid := []byte("version: 1\ntypes:\n  meeting:\n    fields: {}\ntraits: {}\n")
	if err := Write(vaultPath, valid); err != nil {
		t.Fatalf("Write valid schema: %v", err)
	}
	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load valid schema: %v", err)
	}
	if _, ok := loaded.Types["meeting"]; !ok {
		t.Fatal("valid schema write did not add meeting")
	}
}
