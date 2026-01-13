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

	q, err := Parse("object:person .nonexistent==value")
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
		"object:person .name==Freya",
		"object:project .status==active",
		"trait:due",
		"trait:due value==past",
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

func TestValidator_FieldNotTrait(t *testing.T) {
	// Traits are NOT valid as field access - only actual fields are
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":      {},
			"priority": {},
		},
	}

	v := NewValidator(sch)

	// Should be invalid - due is a trait, not a field on project
	q, err := Parse("object:project .due==2025-01-01")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Error("expected validation error for trait used as field, got nil")
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

func TestValidator_SortGroupValidation(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":  {},
			"todo": {},
		},
	}

	v := NewValidator(sch)

	// Valid sort/group queries
	validTests := []string{
		"trait:todo sort:_.value",
		"trait:todo sort:_.parent",
		"trait:todo sort:{trait:due at:_}",
		"trait:todo group:_.parent",
		"trait:todo group:_.refs:project",
		"object:project sort:_.status",
		"object:project sort:min:{trait:due within:_}",
	}

	for _, queryStr := range validTests {
		t.Run("valid: "+queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			if err := v.Validate(q); err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}

	// Invalid sort/group queries
	invalidTests := []struct {
		name        string
		query       string
		wantContain string
	}{
		{
			name:        "_.value on object query",
			query:       "object:project sort:_.value",
			wantContain: "_.value is only valid for trait queries",
		},
		{
			name:        "unknown type in refs path",
			query:       "trait:todo group:_.refs:nonexistent",
			wantContain: "unknown type 'nonexistent'",
		},
		{
			name:        "unknown type in ancestor path",
			query:       "object:project group:_.ancestor:nonexistent",
			wantContain: "unknown type 'nonexistent'",
		},
		{
			name:        "invalid sort subquery type",
			query:       "trait:todo sort:{trait:nonexistent}",
			wantContain: "unknown trait 'nonexistent'",
		},
	}

	for _, tt := range invalidTests {
		t.Run("invalid: "+tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			err = v.Validate(q)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("expected error containing %q, got: %s", tt.wantContain, err.Error())
			}
		})
	}
}

func TestValidator_PipelineSubqueriesMustReferenceSelf(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
			"section": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"todo": {},
			"due":  {},
		},
	}

	v := NewValidator(sch)

	// Valid pipeline queries - subqueries reference _
	validTests := []string{
		"object:project |> todos = count({trait:todo within:_})",
		"object:project |> children = count({object:section parent:_})",
		"object:project |> refs = count({object:project refs:_})",
		"trait:todo |> colocated = count({trait:due at:_})",
		"trait:todo |> refs = count(refs(_))", // NavFunc, not subquery
	}

	for _, queryStr := range validTests {
		t.Run("valid: "+queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			if err := v.Validate(q); err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}

	// Invalid pipeline queries - subqueries DON'T reference _
	invalidTests := []struct {
		name        string
		query       string
		wantContain string
	}{
		{
			name:        "count without self-ref",
			query:       "object:project |> todos = count({trait:todo})",
			wantContain: "must reference _",
		},
		{
			name:        "max without self-ref",
			query:       "object:project |> latest = max({trait:due})",
			wantContain: "must reference _",
		},
		{
			name:        "min without self-ref",
			query:       "object:project |> earliest = min({trait:due})",
			wantContain: "must reference _",
		},
		{
			name:        "subquery with filter but no self-ref",
			query:       "object:project |> active = count({object:section})",
			wantContain: "must reference _",
		},
	}

	for _, tt := range invalidTests {
		t.Run("invalid: "+tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			err = v.Validate(q)
			if err == nil {
				t.Fatal("expected validation error for subquery without _, got nil")
			}

			if !strings.Contains(err.Error(), tt.wantContain) {
				t.Errorf("expected error containing %q, got: %s", tt.wantContain, err.Error())
			}
		})
	}
}

func TestValidator_DirectTargetPredicates(t *testing.T) {
	// Test that [[target]] predicates don't panic and validate correctly
	// This is a regression test for the nil pointer dereference when SubQuery is nil
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
			"section": {Fields: map[string]*schema.FieldDefinition{}},
			"date":    {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"todo": {},
			"due":  {},
		},
	}

	v := NewValidator(sch)

	// All these queries use [[target]] syntax which sets Target instead of SubQuery
	// They should validate without panicking
	tests := []string{
		// Object predicates with [[target]]
		"object:section parent:[[projects/website]]",
		"object:section ancestor:[[projects/website]]",
		"object:project child:[[projects/website#tasks]]",
		"object:project descendant:[[projects/website#tasks]]",
		// Trait predicates with [[target]]
		"trait:todo on:[[projects/website]]",
		"trait:todo within:[[projects/website]]",
		// Negated versions
		"object:section !parent:[[projects/website]]",
		"trait:todo !within:[[projects/website]]",
		// Short references
		"trait:todo within:[[website]]",
	}

	for _, queryStr := range tests {
		t.Run(queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			// This should NOT panic - that's the main test
			err = v.Validate(q)
			if err != nil {
				t.Errorf("unexpected validation error: %v", err)
			}
		})
	}
}
