package cli

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestSchemaFieldPromptText(t *testing.T) {
	tests := []struct {
		name     string
		fieldDef *schema.FieldDefinition
		want     string
	}{
		{
			name:     "optional without default",
			fieldDef: &schema.FieldDefinition{Type: schema.FieldTypeString},
			want:     "email (optional, blank to skip): ",
		},
		{
			name:     "optional with default",
			fieldDef: &schema.FieldDefinition{Type: schema.FieldTypeEnum, Default: "active", Values: []string{"active", "inactive"}},
			want:     "status (optional, enter for active): ",
		},
		{
			name:     "required without default",
			fieldDef: &schema.FieldDefinition{Type: schema.FieldTypeString, Required: true},
			want:     "status (required): ",
		},
		{
			name:     "required with default",
			fieldDef: &schema.FieldDefinition{Type: schema.FieldTypeString, Required: true, Default: "draft"},
			want:     "status (required, enter for draft): ",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fieldName := "status"
			if tc.name == "optional without default" {
				fieldName = "email"
			}
			if got := schemaFieldPromptText(fieldName, tc.fieldDef); got != tc.want {
				t.Fatalf("schemaFieldPromptText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestResolveInteractiveSchemaFieldInput(t *testing.T) {
	fieldDef := &schema.FieldDefinition{Type: schema.FieldTypeString, Default: "draft"}

	value, set, err := resolveInteractiveSchemaFieldInput("status", fieldDef, "\n")
	if err != nil {
		t.Fatalf("resolveInteractiveSchemaFieldInput returned error: %v", err)
	}
	if !set || value != "draft" {
		t.Fatalf("blank input with default: set=%v value=%q, want set=true value=draft", set, value)
	}

	value, set, err = resolveInteractiveSchemaFieldInput("status", fieldDef, "published\n")
	if err != nil {
		t.Fatalf("resolveInteractiveSchemaFieldInput returned error: %v", err)
	}
	if !set || value != "published" {
		t.Fatalf("explicit input: set=%v value=%q, want set=true value=published", set, value)
	}

	optional := &schema.FieldDefinition{Type: schema.FieldTypeString}
	value, set, err = resolveInteractiveSchemaFieldInput("email", optional, "  \n")
	if err != nil {
		t.Fatalf("resolveInteractiveSchemaFieldInput returned error: %v", err)
	}
	if set {
		t.Fatalf("blank optional field without default should not be set, got value=%q", value)
	}

	required := &schema.FieldDefinition{Type: schema.FieldTypeString, Required: true}
	_, _, err = resolveInteractiveSchemaFieldInput("status", required, "\n")
	if err == nil {
		t.Fatal("expected blank required field without default to error")
	}
}
