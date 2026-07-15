package parser

import (
	"testing"

	"github.com/aidanlsb/raven/internal/schema"

	"github.com/aidanlsb/raven/internal/model"
)

func TestExtractSchemaFieldRefs(t *testing.T) {
	t.Parallel()

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				Fields: map[string]*schema.FieldDefinition{
					"company": {Type: schema.FieldTypeRef, Target: "company"},
					"tags":    {Type: schema.FieldTypeRefArray, Target: "tag"},
					"name":    {Type: schema.FieldTypeString},
				},
			},
			"company": {Fields: map[string]*schema.FieldDefinition{}},
			"tag":     {Fields: map[string]*schema.FieldDefinition{}},
		},
	}

	objects := []*model.Object{{
		ID:        "person/ada",
		Type:      "person",
		LineStart: 3,
		Fields: map[string]schema.FieldValue{
			"name":    schema.String("Ada"),
			"company": schema.String("cursor"),
			"tags":    schema.Array([]schema.FieldValue{schema.String("ai"), schema.String("tools")}),
		},
	}}

	refs := ExtractSchemaFieldRefs(objects, sch)
	if len(refs) != 3 {
		t.Fatalf("refs = %#v, want 3", refs)
	}

	got := map[string]SchemaFieldRef{}
	for _, ref := range refs {
		got[ref.FieldName+"|"+ref.TargetRaw] = ref
	}
	for _, want := range []SchemaFieldRef{
		{SourceID: "person/ada", FieldName: "company", TargetRaw: "cursor", Line: 3},
		{SourceID: "person/ada", FieldName: "tags", TargetRaw: "ai", Line: 3},
		{SourceID: "person/ada", FieldName: "tags", TargetRaw: "tools", Line: 3},
	} {
		key := want.FieldName + "|" + want.TargetRaw
		gotRef, ok := got[key]
		if !ok {
			t.Fatalf("missing ref %s in %#v", key, refs)
		}
		if gotRef != want {
			t.Fatalf("ref %s = %#v, want %#v", key, gotRef, want)
		}
	}
}

func TestSchemaFieldRefsAsReferences(t *testing.T) {
	t.Parallel()

	refs := SchemaFieldRefsAsReferences([]SchemaFieldRef{{
		SourceID:  "person/ada",
		FieldName: "company",
		TargetRaw: "cursor",
		Line:      4,
	}})
	if len(refs) != 1 {
		t.Fatalf("len = %d, want 1", len(refs))
	}
	if refs[0].SourceID != "person/ada" || refs[0].TargetRaw != "cursor" || refs[0].LineOrZero() != 4 {
		t.Fatalf("ref = %#v", refs[0])
	}
}
