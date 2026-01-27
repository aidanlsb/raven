package query

import (
	"errors"
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

func TestValidator_SelfRefOutsidePipelineRejected(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {},
		},
	}

	v := NewValidator(sch)

	q, err := Parse("object:project parent:_")
	if err != nil {
		t.Fatalf("failed to parse query: %v", err)
	}

	err = v.Validate(q)
	if err == nil {
		t.Fatal("expected validation error for self-reference outside pipeline")
	}
	if !strings.Contains(err.Error(), "self-reference '_'") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidator_TraitRefdRejected(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due": {},
		},
	}

	v := NewValidator(sch)

	q, err := Parse("trait:due refd:[[projects/website]]")
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

	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("expected ValidationError, got %T", err)
	}

	if !strings.Contains(ve.Message, "unknown trait 'nonexistent'") {
		t.Errorf("expected error about unknown trait, got: %s", ve.Message)
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
			query:       "object:project |> latest = max(.value, {trait:due})",
			wantContain: "must reference _",
		},
		{
			name:        "min without self-ref",
			query:       "object:project |> earliest = min(.value, {trait:due})",
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

func TestValidator_PipelineAggregationTyping(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"priority": {Type: schema.FieldTypeNumber},
					"name":     {Type: schema.FieldTypeString},
					"tags":     {Type: schema.FieldTypeStringArray},
					"owner":    {Type: schema.FieldTypeRef, Target: "person"},
				},
			},
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":   {Type: schema.FieldTypeDate},
			"score": {Type: schema.FieldTypeNumber},
			"flag":  {Type: schema.FieldTypeBool}, // boolean trait (no value)
		},
	}

	v := NewValidator(sch)

	valid := []string{
		"object:project |> todos = count({trait:due within:_})",
		"object:project |> earliest = min(.value, {trait:due within:_})",
		"object:project |> maxPriority = max(.priority, {object:project refs:_})",
		"trait:due |> refs = count(refs(_))",
	}
	for _, qs := range valid {
		t.Run("valid: "+qs, func(t *testing.T) {
			q, err := Parse(qs)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}
			if err := v.Validate(q); err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
		})
	}

	invalid := []struct {
		name        string
		query       string
		wantContain string
	}{
		{
			name:        "nav function only supports count",
			query:       "object:project |> x = max(refs(_))",
			wantContain: "navigation functions only support count()",
		},
		{
			name:        "count does not accept field arg",
			query:       "object:project |> x = count(.priority, {object:project refs:_})",
			wantContain: "count() does not accept a field argument",
		},
		{
			name:        "min requires field arg",
			query:       "object:project |> x = min({trait:due within:_})",
			wantContain: "requires a field argument",
		},
		{
			name:        "trait aggregates require .value",
			query:       "object:project |> x = max(.priority, {trait:due within:_})",
			wantContain: "only supports .value",
		},
		{
			name:        "sum on non-numeric trait is rejected",
			query:       "object:project |> x = sum(.value, {trait:due within:_})",
			wantContain: "requires a numeric trait",
		},
		{
			name:        "min/max on boolean trait is rejected",
			query:       "object:project |> x = max(.value, {trait:flag within:_})",
			wantContain: "boolean trait",
		},
		{
			name:        "object aggregates reject array fields",
			query:       "object:project |> x = min(.tags, {object:project refs:_})",
			wantContain: "cannot use min()",
		},
		{
			name:        "object aggregates reject ref fields",
			query:       "object:project |> x = max(.owner, {object:project refs:_})",
			wantContain: "cannot use max()",
		},
		{
			name:        "object aggregates validate field exists",
			query:       "object:project |> x = max(.missing, {object:project refs:_})",
			wantContain: "has no field 'missing'",
		},
		{
			name:        "sum on object requires numeric field type",
			query:       "object:project |> x = sum(.name, {object:project refs:_})",
			wantContain: "cannot use sum()",
		},
	}

	for _, tc := range invalid {
		t.Run("invalid: "+tc.name, func(t *testing.T) {
			q, err := Parse(tc.query)
			if err != nil {
				t.Fatalf("failed to parse: %v", err)
			}
			err = v.Validate(q)
			if err == nil {
				t.Fatalf("expected validation error")
			}
			if !strings.Contains(err.Error(), tc.wantContain) {
				t.Fatalf("expected error containing %q, got %v", tc.wantContain, err)
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
