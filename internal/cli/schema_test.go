package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/commands"
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

func TestBuildTypeSchemaIncludesTemplateBindings(t *testing.T) {
	typeDef := &schema.TypeDefinition{
		Templates:       []string{"interview_technical"},
		DefaultTemplate: "interview_technical",
	}

	result := buildTypeSchema("interview", typeDef, false)

	if len(result.Templates) != 1 || result.Templates[0] != "interview_technical" {
		t.Fatalf("expected type templates [%q], got %v", "interview_technical", result.Templates)
	}
	if result.DefaultTemplate != "interview_technical" {
		t.Fatalf("expected default template %q, got %q", "interview_technical", result.DefaultTemplate)
	}
}

func TestBuildSchemaCommandsIncludesOnlySchemaCommands(t *testing.T) {
	cmds := buildSchemaCommands()

	if _, ok := cmds["schema"]; !ok {
		t.Fatalf("expected schema command to be present")
	}
	if _, ok := cmds["schema_add_type"]; !ok {
		t.Fatalf("expected schema_add_type command to be present")
	}
	if _, ok := cmds["search"]; ok {
		t.Fatalf("expected non-schema command %q to be absent", "search")
	}
	if len(cmds) >= len(commands.Registry) {
		t.Fatalf("expected schema command list to be a filtered subset: got %d of %d", len(cmds), len(commands.Registry))
	}
}
