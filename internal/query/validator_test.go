package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidator_UnknownType(t *testing.T) {
	t.Parallel()
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

	var ve *ValidationError
	if !errors.As(err, &ve) {
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
	t.Parallel()
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

	var ve *ValidationError
	if !errors.As(err, &ve) {
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
	t.Parallel()
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

	var ve *ValidationError
	if !errors.As(err, &ve) {
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
	t.Parallel()
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
		"trait:due .value==past",
		"object:person has(trait:due)",
		"trait:due on(object:project)",
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

func TestValidator_TraitRefdRejected(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {},
		},
	}

	v := NewValidator(sch)

	q, err := Parse("trait:due refd([[projects/website]])")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for refd in trait query")
	}
	if !strings.Contains(err.Error(), "refd:") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidator_FieldNotTrait(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	q, err := Parse("object:project has(trait:nonexistent)")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for unknown trait in subquery")
	}

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "unknown trait 'nonexistent'") {
		t.Errorf("expected error about unknown trait, got: %s", ve.Message)
	}
}

func TestValidator_DirectTargetPredicates(t *testing.T) {
	t.Parallel()
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
		"object:section parent([[projects/website]])",
		"object:section ancestor([[projects/website]])",
		"object:project child([[projects/website#tasks]])",
		"object:project descendant([[projects/website#tasks]])",
		// Trait predicates with [[target]]
		"trait:todo on([[projects/website]])",
		"trait:todo within([[projects/website]])",
		// Negated versions
		"object:section !parent([[projects/website]])",
		"trait:todo !within([[projects/website]])",
		// Short references
		"trait:todo within([[website]])",
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

func TestValidator_TraitStringFunctionsRequireValueField(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{},
		Traits: map[string]*schema.TraitDefinition{
			"todo": {},
		},
	}

	v := NewValidator(sch)

	tests := []struct {
		name    string
		query   string
		wantErr bool
	}{
		{
			name:    "value field is allowed",
			query:   `trait:todo contains(.value, "todo")`,
			wantErr: false,
		},
		{
			name:    "content field is rejected",
			query:   `trait:todo contains(.content, "todo")`,
			wantErr: true,
		},
		{
			name:    "element placeholder is rejected",
			query:   `trait:todo contains(_, "todo")`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			err = v.Validate(q)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}
				if !strings.Contains(err.Error(), "only support .value") {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}

func TestValidator_ObjectStringFunctionsAndArrayQuantifiersValidateFieldTypes(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"name":   {Type: schema.FieldTypeString},
					"score":  {Type: schema.FieldTypeNumber},
					"status": {Type: schema.FieldTypeEnum},
					"tags":   {Type: schema.FieldTypeStringArray},
					"scores": {Type: schema.FieldTypeNumberArray},
					"owners": {Type: schema.FieldTypeRefArray, Target: "person"},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	v := NewValidator(sch)

	tests := []struct {
		name        string
		query       string
		wantErr     bool
		errContains string
	}{
		{
			name:    "string function on scalar string field",
			query:   `object:project contains(.name, "api")`,
			wantErr: false,
		},
		{
			name:    "array quantifier on string array with element string function",
			query:   `object:project any(.tags, startswith(_, "feature-"))`,
			wantErr: false,
		},
		{
			name:    "array quantifier on ref array with ref element comparison",
			query:   `object:project any(.owners, _ == [[people/freya]])`,
			wantErr: false,
		},
		{
			name:        "string function unknown field",
			query:       `object:project contains(.missing, "api")`,
			wantErr:     true,
			errContains: "has no field 'missing'",
		},
		{
			name:        "string function on number field",
			query:       `object:project contains(.score, "9")`,
			wantErr:     true,
			errContains: "not valid for field '.score'",
		},
		{
			name:        "string function on array field",
			query:       `object:project contains(.tags, "urgent")`,
			wantErr:     true,
			errContains: "require a scalar field",
		},
		{
			name:        "top-level string function underscore placeholder",
			query:       `object:project contains(_, "api")`,
			wantErr:     true,
			errContains: "placeholder '_' is only valid inside any()/all()/none()",
		},
		{
			name:        "array quantifier on non-array field",
			query:       `object:project any(.name, _ == "api")`,
			wantErr:     true,
			errContains: "require an array field",
		},
		{
			name:        "array element string function on numeric array",
			query:       `object:project any(.scores, startswith(_, "1"))`,
			wantErr:     true,
			errContains: "not valid for array elements of type number",
		},
		{
			name:        "array element string function must use underscore",
			query:       `object:project any(.tags, startswith(.name, "api"))`,
			wantErr:     true,
			errContains: "must use '_' as the first argument",
		},
		{
			name:        "reference element comparison only for ref arrays",
			query:       `object:project any(.tags, _ == [[people/freya]])`,
			wantErr:     true,
			errContains: "only valid for ref[] fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			err = v.Validate(q)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected validation error, got nil")
				}
				if tt.errContains != "" && !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("expected error containing %q, got: %v", tt.errContains, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}
}
