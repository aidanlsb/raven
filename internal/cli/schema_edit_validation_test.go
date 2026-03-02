package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidateFieldTypeSpecAcceptsDateFieldTypes(t *testing.T) {
	sch := schema.NewSchema()

	tests := []struct {
		name      string
		fieldType string
		isArray   bool
	}{
		{
			name:      "date",
			fieldType: "date",
			isArray:   false,
		},
		{
			name:      "date array",
			fieldType: "date[]",
			isArray:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := validateFieldTypeSpec(tt.fieldType, "", "", sch)
			if !got.Valid {
				t.Fatalf("expected %q to be valid, got error: %s", tt.fieldType, got.Error)
			}
			if got.BaseType != "date" {
				t.Fatalf("expected base type %q, got %q", "date", got.BaseType)
			}
			if got.IsArray != tt.isArray {
				t.Fatalf("expected IsArray=%v, got %v", tt.isArray, got.IsArray)
			}
		})
	}
}

func TestValidateFieldTypeSpecRejectsSchemaTypeName(t *testing.T) {
	sch := schema.NewSchema()
	sch.Types["person"] = &schema.TypeDefinition{}

	got := validateFieldTypeSpec("person", "", "", sch)
	if got.Valid {
		t.Fatal("expected schema type name to be rejected as field type")
	}
	if !strings.Contains(got.Error, "type name, not a field type") {
		t.Fatalf("expected type-name error, got %q", got.Error)
	}
}
