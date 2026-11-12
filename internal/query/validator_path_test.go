package query

import (
	"errors"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func relationshipSchema() *schema.Schema {
	sch := schema.New()
	sch.Types["company"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
			"tier": {Type: schema.FieldTypeNumber},
		},
	}
	sch.Types["project"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name":    {Type: schema.FieldTypeString},
			"status":  {Type: schema.FieldTypeString},
			"company": {Type: schema.FieldTypeRef, Target: "company"},
		},
	}
	sch.Types["person"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"name": {Type: schema.FieldTypeString},
			"team": {Type: schema.FieldTypeString},
			"lead": {Type: schema.FieldTypeRef, Target: "person"},
		},
	}
	sch.Types["task"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"status":        {Type: schema.FieldTypeString},
			"due":           {Type: schema.FieldTypeDate},
			"created":       {Type: schema.FieldTypeDate},
			"project":       {Type: schema.FieldTypeRef, Target: "project"},
			"owner":         {Type: schema.FieldTypeRef, Target: "person"},
			"collaborators": {Type: schema.FieldTypeRefArray, Target: "person"},
		},
	}
	sch.Traits["related"] = &schema.TraitDefinition{Type: schema.FieldTypeRef}
	return sch
}

func TestValidator_FieldPaths(t *testing.T) {
	t.Parallel()
	v := NewValidator(relationshipSchema())

	tests := []struct {
		query       string
		wantErr     string
		wantSuggest string
	}{
		{query: "type:task .project.status==active"},
		{query: "type:task .project.company.tier==1"},
		{query: "type:task .project==[[project/foo]]"},
		{query: `type:task includes(.project.name, "web")`},
		{query: "type:task exists(.project.name)"},
		{query: "type:task oneof(.project.status, [active, backlog])"},
		{query: "type:task any(.collaborators, .team==infra)"},
		{query: "type:person refd(type:task .owner==this .status==open)"},
		{query: "type:person refd(type:task this.lead==ada)"},
		{query: "type:company refd(type:task .project.company==this)"},
		{query: "type:person refd(type:task any(.collaborators, _==this))"},
		{query: "type:task .due>.created"},
		{
			query:   "type:task .owner==this",
			wantErr: "only valid in a nested query",
		},
		{
			query:   "type:task this.status==open",
			wantErr: "only valid in a nested query",
		},
		{
			query:       "type:task .status.active==yes",
			wantErr:     "cannot follow 'active' on field 'status' of type string",
			wantSuggest: "Named paths may only hop through scalar ref fields",
		},
		{
			query:   "type:task any(.collaborators, .missing==x)",
			wantErr: "has no field 'missing'",
		},
		{
			query:   "type:task any(.status, .team==infra)",
			wantErr: "require an array field",
		},
		{
			query:   `link .ext==.pdf`,
			wantErr: "is a path, not a literal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			q, err := Parse(tt.query)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			err = v.Validate(q)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate: %v", err)
				}
				return
			}
			var ve *ValidationError
			if !errors.As(err, &ve) {
				t.Fatalf("error = %T %v, want ValidationError", err, err)
			}
			if !strings.Contains(ve.Message, tt.wantErr) {
				t.Fatalf("message = %q, want substring %q", ve.Message, tt.wantErr)
			}
			if tt.wantSuggest != "" && !strings.Contains(ve.Suggestion, tt.wantSuggest) {
				t.Fatalf("suggestion = %q, want substring %q", ve.Suggestion, tt.wantSuggest)
			}
		})
	}
}

func TestValidator_TopLevelThisWithoutSchema(t *testing.T) {
	t.Parallel()
	q, err := Parse("type:task .owner==this")
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	err = NewValidator(nil).Validate(q)
	var ve *ValidationError
	if !errors.As(err, &ve) {
		t.Fatalf("error = %T %v", err, err)
	}
	if !strings.Contains(ve.Message, "only valid in a nested query") {
		t.Fatalf("message = %q", ve.Message)
	}
}
