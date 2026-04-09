package schemasvc

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidateFieldTypeSpecAcceptsDateFieldTypesAndAliases(t *testing.T) {
	sch := schema.New()

	tests := []struct {
		name      string
		fieldType string
		baseType  string
		isArray   bool
	}{
		{
			name:      "date",
			fieldType: "date",
			baseType:  "date",
			isArray:   false,
		},
		{
			name:      "date array",
			fieldType: "date[]",
			baseType:  "date",
			isArray:   true,
		},
		{
			name:      "boolean alias",
			fieldType: "boolean",
			baseType:  "bool",
			isArray:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateFieldTypeSpec(tt.fieldType, "", "", sch)
			if !got.Valid {
				t.Fatalf("expected %q to be valid, got error: %s", tt.fieldType, got.Error)
			}
			if got.BaseType != tt.baseType {
				t.Fatalf("expected base type %q, got %q", tt.baseType, got.BaseType)
			}
			if got.IsArray != tt.isArray {
				t.Fatalf("expected IsArray=%v, got %v", tt.isArray, got.IsArray)
			}
		})
	}
}

func TestValidateFieldTypeSpecRejectsSchemaTypeName(t *testing.T) {
	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{}

	got := ValidateFieldTypeSpec("person", "", "", sch)
	if got.Valid {
		t.Fatal("expected schema type name to be rejected as field type")
	}
	if !strings.Contains(got.Error, "type name, not a field type") {
		t.Fatalf("expected type-name error, got %q", got.Error)
	}
}
