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
			"zebra":  {Fields: map[string]*schema.FieldDefinition{}},
			"alpha":  {Fields: map[string]*schema.FieldDefinition{}},
			"middle": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	v := NewValidator(sch)

	q, err := Parse("type:nonexistent")
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

	if ve.Suggestion != "Available types: alpha, middle, zebra" {
		t.Errorf("suggestion = %q, want sorted available types", ve.Suggestion)
	}
}

func TestValidator_NilQuery(t *testing.T) {
	t.Parallel()

	v := NewValidator(schema.New())
	if err := v.Validate(nil); err != nil {
		t.Fatalf("Validate(nil) returned error: %v", err)
	}
}

func TestValidator_UnknownTrait(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{},
		Traits: map[string]*schema.TraitDefinition{
			"zeta":  {},
			"alpha": {},
			"mid":   {},
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

	if ve.Suggestion != "Available traits: alpha, mid, zeta" {
		t.Errorf("suggestion = %q, want sorted available traits", ve.Suggestion)
	}
}

func TestValidator_UnknownField(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"zeta":  {Type: schema.FieldTypeString},
					"alpha": {Type: schema.FieldTypeString},
					"mid":   {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{},
	}

	v := NewValidator(sch)

	q, err := Parse("type:person .nonexistent==value")
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

	if ve.Suggestion != "Available fields: alpha, mid, zeta" {
		t.Errorf("suggestion = %q, want sorted available fields", ve.Suggestion)
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
			"date": {
				Fields: map[string]*schema.FieldDefinition{},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":      {},
			"priority": {},
		},
	}

	v := NewValidator(sch)

	tests := []string{
		"type:person",
		"type:person .name==Freya",
		"type:project .status==active",
		"type:date .date>=2026-05-01",
		"trait:due",
		"trait:due .value==past",
		"type:person has(trait:due)",
		"trait:due in(type:project)",
		"asset",
		"asset .extension==pdf",
		`asset startswith(.media_type, "image/")`,
		"asset .size_bytes>1024",
		"asset refd(type:project .status==active)",
		"asset refd(trait:due)",
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

func TestValidator_AssetQueryRules(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"status": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"todo": {},
		},
	}
	v := NewValidator(sch)

	tests := []struct {
		name    string
		query   string
		wantMsg string
	}{
		{
			name:    "unknown field",
			query:   "asset .status==active",
			wantMsg: "asset has no field 'status'",
		},
		{
			name:    "string function on number",
			query:   `asset includes(.size_bytes, "12")`,
			wantMsg: "string function predicates are not valid for asset field '.size_bytes'",
		},
		{
			name:    "refs rejected",
			query:   "asset refs([[project/raven]])",
			wantMsg: "refs() predicate is not valid for asset queries",
		},
		{
			name:    "content rejected",
			query:   `asset content("diagram")`,
			wantMsg: "content() predicate is not valid for asset queries",
		},
		{
			name:    "has rejected",
			query:   "asset has(trait:todo)",
			wantMsg: "has() predicate is not valid for asset queries",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}
			err = v.Validate(q)
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.wantMsg) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tt.wantMsg)
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
	if !strings.Contains(err.Error(), "refd()") {
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
	q, err := Parse("type:project .due==2025-01-01")
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
	q, err := Parse("type:project has(trait:nonexistent)")
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

func TestValidator_InvalidRegexPattern(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
		Traits: map[string]*schema.TraitDefinition{
			"todo": {},
		},
	}

	v := NewValidator(sch)

	tests := []string{
		`type:project matches(.name, "[")`,
		`trait:todo matches(.value, "[")`,
	}

	for _, queryStr := range tests {
		t.Run(queryStr, func(t *testing.T) {
			q, err := Parse(queryStr)
			if err != nil {
				t.Fatalf("failed to parse query: %v", err)
			}

			err = v.Validate(q)
			if err == nil {
				t.Fatal("expected validation error for invalid regex")
			}

			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("expected ValidationError, got %T", err)
			}
			if !strings.Contains(ve.Message, "invalid regex pattern") {
				t.Fatalf("unexpected validation message: %q", ve.Message)
			}
		})
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
			"date": {Fields: map[string]*schema.FieldDefinition{}},
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
		// Section predicates with [[target]]
		"section in([[projects/website]])",
		"section within([[projects/website]])",
		"type:project has(section .id==projects/website#tasks)",
		"type:project contains(section .id==projects/website#tasks)",
		// Trait predicates with [[target]]
		"trait:todo in([[projects/website]])",
		"trait:todo within([[projects/website]])",
		// Negated versions
		"section !in([[projects/website]])",
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
			query:   `trait:todo includes(.value, "todo")`,
			wantErr: false,
		},
		{
			name:    "content field is rejected",
			query:   `trait:todo includes(.content, "todo")`,
			wantErr: true,
		},
		{
			name:    "element placeholder is rejected",
			query:   `trait:todo includes(_, "todo")`,
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
			query:   `type:project includes(.name, "api")`,
			wantErr: false,
		},
		{
			name:    "array quantifier on string array with element string function",
			query:   `type:project any(.tags, startswith(_, "feature-"))`,
			wantErr: false,
		},
		{
			name:    "array quantifier on ref array with ref element comparison",
			query:   `type:project any(.owners, _ == [[people/freya]])`,
			wantErr: false,
		},
		{
			name:        "string function unknown field",
			query:       `type:project includes(.missing, "api")`,
			wantErr:     true,
			errContains: "has no field 'missing'",
		},
		{
			name:        "string function on number field",
			query:       `type:project includes(.score, "9")`,
			wantErr:     true,
			errContains: "not valid for field '.score'",
		},
		{
			name:        "string function on array field",
			query:       `type:project includes(.tags, "urgent")`,
			wantErr:     true,
			errContains: "require a scalar field",
		},
		{
			name:        "top-level string function underscore placeholder",
			query:       `type:project includes(_, "api")`,
			wantErr:     true,
			errContains: "placeholder '_' is only valid inside any()/all()/none()",
		},
		{
			name:        "array quantifier on non-array field",
			query:       `type:project any(.name, _ == "api")`,
			wantErr:     true,
			errContains: "require an array field",
		},
		{
			name:        "array element string function on numeric array",
			query:       `type:project any(.scores, startswith(_, "1"))`,
			wantErr:     true,
			errContains: "not valid for array elements of type number",
		},
		{
			name:        "array element string function must use underscore",
			query:       `type:project any(.tags, startswith(.name, "api"))`,
			wantErr:     true,
			errContains: "must use '_' as the first argument",
		},
		{
			name:        "reference element comparison only for ref arrays",
			query:       `type:project any(.tags, _ == [[people/freya]])`,
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

func TestValidator_TraitArrayQuantifiersValidateValueType(t *testing.T) {
	t.Parallel()
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{},
		Traits: map[string]*schema.TraitDefinition{
			"tags":      {Type: schema.FieldTypeStringArray},
			"scores":    {Type: schema.FieldTypeNumberArray},
			"reviewers": {Type: schema.FieldTypeRefArray},
			"todo":      {Type: schema.FieldTypeString},
		},
	}

	v := NewValidator(sch)
	tests := []struct {
		name        string
		query       string
		wantErr     bool
		errContains string
	}{
		{
			name:  "array-valued trait allows any on value",
			query: `trait:tags any(.value, startswith(_, "rav"))`,
		},
		{
			name:  "ref array trait allows ref element comparison",
			query: `trait:reviewers any(.value, _ == [[people/freya]])`,
		},
		{
			name:        "scalar trait rejects array predicate",
			query:       `trait:todo any(.value, _ == todo)`,
			wantErr:     true,
			errContains: "require an array-valued trait",
		},
		{
			name:        "array trait only supports value field",
			query:       `trait:tags any(.content, _ == raven)`,
			wantErr:     true,
			errContains: "only support .value",
		},
		{
			name:        "numeric array rejects string function",
			query:       `trait:scores any(.value, startswith(_, "1"))`,
			wantErr:     true,
			errContains: "not valid for array elements of type number",
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
