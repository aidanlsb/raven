package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestValidator_SortFields(t *testing.T) {
	t.Parallel()

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"title":    {Type: schema.FieldTypeString},
					"status":   {Type: schema.FieldTypeEnum},
					"priority": {Type: schema.FieldTypeNumber},
					"tags":     {Type: schema.FieldTypeStringArray},
				},
			},
			"date": {Fields: map[string]*schema.FieldDefinition{}},
		},
		Traits: map[string]*schema.TraitDefinition{
			"due":  {Type: schema.FieldTypeDate},
			"tags": {Type: schema.FieldTypeStringArray},
		},
	}
	v := NewValidator(sch)

	tests := []struct {
		name        string
		query       string
		wantErr     bool
		errContains string
		suggestion  string
	}{
		{
			name:    "object scalar field",
			query:   "type:project sort(.title, desc)",
			wantErr: false,
		},
		{
			name:    "object number field",
			query:   "type:project sort(.priority)",
			wantErr: false,
		},
		{
			name:    "date virtual field",
			query:   "type:date sort(.date, desc)",
			wantErr: false,
		},
		{
			name:    "multi-field",
			query:   "type:project sort(.priority, desc) sort(.title, asc)",
			wantErr: false,
		},
		{
			name:        "unknown object field",
			query:       "type:project sort(.missing)",
			wantErr:     true,
			errContains: "type 'project' has no field 'missing'",
		},
		{
			name:        "array object field",
			query:       "type:project sort(.tags)",
			wantErr:     true,
			errContains: "cannot sort by array field '.tags' of type string[]",
			suggestion:  "Sort by a scalar field instead",
		},
		{
			name:        "duplicate sort field",
			query:       "type:project sort(.title) sort(.title, desc)",
			wantErr:     true,
			errContains: "duplicate sort field '.title'",
		},
		{
			name:    "trait .value",
			query:   "trait:due sort(.value, desc)",
			wantErr: false,
		},
		{
			name:        "trait non-value field",
			query:       "trait:due sort(.status)",
			wantErr:     true,
			errContains: "sort() on trait queries only supports .value, got .status",
			suggestion:  "Use sort(.value) or sort(.value, desc)",
		},
		{
			name:        "array-valued trait",
			query:       "trait:tags sort(.value)",
			wantErr:     true,
			errContains: "cannot sort by array-valued trait 'tags'",
		},
		{
			name:    "section known field",
			query:   "section sort(.title)",
			wantErr: false,
		},
		{
			name:    "section numeric field",
			query:   "section sort(.level, desc)",
			wantErr: false,
		},
		{
			name:        "section unknown field",
			query:       "section sort(.status)",
			wantErr:     true,
			errContains: "section has no field 'status'",
			suggestion:  "Available section fields:",
		},
		{
			name:    "link known field",
			query:   "link sort(.ext)",
			wantErr: false,
		},
		{
			name:    "link numeric field",
			query:   "link sort(.line, desc)",
			wantErr: false,
		},
		{
			name:        "link unknown field",
			query:       "link sort(.title)",
			wantErr:     true,
			errContains: "link has no field 'title'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse(%q): %v", tt.query, err)
			}
			err = v.Validate(q)
			if !tt.wantErr {
				if err != nil {
					t.Fatalf("Validate() unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected validation error, got nil")
			}
			if !strings.Contains(err.Error(), tt.errContains) {
				t.Fatalf("error = %q, want substring %q", err, tt.errContains)
			}
			if tt.suggestion != "" {
				var ve *ValidationError
				if !errors.As(err, &ve) {
					t.Fatalf("expected ValidationError, got %T", err)
				}
				if !strings.Contains(ve.Suggestion, tt.suggestion) {
					t.Fatalf("suggestion = %q, want substring %q", ve.Suggestion, tt.suggestion)
				}
			}
		})
	}
}

func TestValidator_SortWithoutSchemaAllowsUnknownObjectFields(t *testing.T) {
	t.Parallel()

	q, err := Parse("type:project sort(.anything)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	if err := NewValidator(nil).Validate(q); err != nil {
		t.Fatalf("schema-less Validate() = %v", err)
	}
}

func TestValidator_SortWithoutSchemaStillRejectsTraitNonValue(t *testing.T) {
	t.Parallel()

	q, err := Parse("trait:due sort(.status)")
	if err != nil {
		t.Fatalf("Parse(): %v", err)
	}
	err = NewValidator(nil).Validate(q)
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !strings.Contains(err.Error(), "only supports .value") {
		t.Fatalf("error = %q", err)
	}
}
