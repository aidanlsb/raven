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
	if !strings.Contains(got.Suggestion, "--type ref --target person") {
		t.Fatalf("expected --type ref --target suggestion, got %q", got.Suggestion)
	}
}

func TestValidateFieldTypeSpecCLIConstraints(t *testing.T) {
	sch := schema.New()
	sch.Types["person"] = &schema.TypeDefinition{}

	tests := []struct {
		name      string
		fieldType string
		target    string
		values    string
		wantValid bool
		wantMsg   string
	}{
		{
			name:      "ref without target",
			fieldType: "ref",
			wantMsg:   "--target",
		},
		{
			name:      "ref array without target keeps flag copy",
			fieldType: "ref[]",
			wantMsg:   "--target",
		},
		{
			name:      "enum without values",
			fieldType: "enum",
			wantMsg:   "--values",
		},
		{
			name:      "target on string",
			fieldType: "string",
			target:    "person",
			wantMsg:   "--target is only valid for ref fields",
		},
		{
			name:      "unknown type",
			fieldType: "mystery",
			wantMsg:   "not a valid field type",
		},
		{
			name:      "ref with target",
			fieldType: "ref",
			target:    "person",
			wantValid: true,
		},
		{
			name:      "enum with values",
			fieldType: "enum",
			values:    "a,b",
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ValidateFieldTypeSpec(tt.fieldType, tt.target, tt.values, sch)
			if got.Valid != tt.wantValid {
				t.Fatalf("Valid = %v, want %v (error=%q)", got.Valid, tt.wantValid, got.Error)
			}
			if !tt.wantValid && !strings.Contains(got.Error, tt.wantMsg) {
				t.Fatalf("error %q does not contain %q", got.Error, tt.wantMsg)
			}
		})
	}
}
