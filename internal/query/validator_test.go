package query

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidator_UnknownType(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person":  {Fields: map[string]*schema.FieldDefinition{}},
			"project": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	v := NewValidator(sch)

	q, err := Parse("object:nonexistent")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for unknown type")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "unknown type 'nonexistent'") {
		t.Errorf("expected error about unknown type, got: %s", ve.Message)
	}

	if !strings.Contains(ve.Suggestion, "person") {
		t.Errorf("expected suggestion to include available types, got: %s", ve.Suggestion)
	}
}

func TestValidator_UnknownTrait(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{},
		Traits: map[string]*schema.TraitDefinition{
			"due":      {},
			"priority": {},
		},
	}

	v := NewValidator(sch)

	q, err := Parse("trait:nonexistent")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for unknown trait")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "unknown trait 'nonexistent'") {
		t.Errorf("expected error about unknown trait, got: %s", ve.Message)
	}

	if !strings.Contains(ve.Suggestion, "due") {
		t.Errorf("expected suggestion to include available traits, got: %s", ve.Suggestion)
	}
}

func TestValidator_UnknownField(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	v := NewValidator(sch)

	q, err := Parse("object:person .nonexistent:value")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for unknown field")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "has no field 'nonexistent'") {
		t.Errorf("expected error about unknown field, got: %s", ve.Message)
	}

	if !strings.Contains(ve.Suggestion, "name") {
		t.Errorf("expected suggestion to include available fields, got: %s", ve.Suggestion)
	}
}

func TestValidator_ValidQuery(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":      {},
			"priority": {},
		},
	}

	v := NewValidator(sch)

	tests := []string{
		"object:person",
		"object:person .name:Freya",
		"object:project .status:active",
		"trait:due",
		"trait:due value:past",
		"object:person has:{trait:due}",
		"trait:due on:{object:project}",
	}

	for _, queryStr := range tests {
		t.Run(queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			if err := v.Validate(q); err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidator_FrontmatterTraitAsField(t *testing.T) {
	// Traits declared on a type should be valid as field access
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
				Traits: schema.TypeTraits{
					Configs: map[string]*schema.TypeTraitConfig{
						"due":      {},
						"priority": {},
					},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":      {},
			"priority": {},
		},
	}

	v := NewValidator(sch)

	// Should be valid - due is a trait on the type
	q, err := Parse("object:project .due:2025-01-01")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	if err := v.Validate(q); err != nil {
		t.Errorf("unexpected validation error for frontmatter trait: %v", err)
	}
}

func TestValidator_NestedSubqueryValidation(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {},
		},
	}

	v := NewValidator(sch)

	// Invalid nested subquery - trait doesn't exist
	q, err := Parse("object:project has:{trait:nonexistent}")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for unknown trait in subquery")
	}

	ve, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "unknown trait 'nonexistent'") {
		t.Errorf("expected error about unknown trait, got: %s", ve.Message)
	}
}
