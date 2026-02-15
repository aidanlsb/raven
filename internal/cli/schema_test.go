package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestBuildTypeSchemaIncludesDescriptions(t *testing.T) {
	typeDef := &schema.TypeDefinition{
		Description: "People and contacts",
		Fields: map[string]*schema.FieldDefinition{
			"name": {
				Type:        schema.FieldTypeString,
				Required:    true,
				Description: "Full display name",
			},
		},
	}

	result := buildTypeSchema("person", typeDef, false)

	if result.Description != "People and contacts" {
		t.Fatalf("expected type description %q, got %q", "People and contacts", result.Description)
	}

	field, ok := result.Fields["name"]
	if !ok {
		t.Fatal("expected name field in schema result")
	}
	if field.Description != "Full display name" {
		t.Fatalf("expected field description %q, got %q", "Full display name", field.Description)
	}
}
