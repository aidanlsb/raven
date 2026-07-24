package parser

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
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
		Fields: map[string]fieldvalue.FieldValue{
			"name":    fieldvalue.String("Ada"),
			"company": fieldvalue.String("cursor"),
			"tags":    fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("ai"), fieldvalue.String("tools")}),
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

func TestSchemaFieldRefAtPosition(t *testing.T) {
	t.Parallel()

	content := `---
type: project
title: people/freya
owner: "people/freya"
reviewers:
  - people/loki
  - 'people/freya'
approvers: [people/freya, "people/loki"]
---
`
	doc, err := ParseDocument(content, "/vault/projects/navigation.md", "/vault")
	if err != nil {
		t.Fatal(err)
	}
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"project": {
				Fields: map[string]*schema.FieldDefinition{
					"title":     {Type: schema.FieldTypeString},
					"owner":     {Type: schema.FieldTypeRef, Target: "person"},
					"reviewers": {Type: schema.FieldTypeRefArray, Target: "person"},
					"approvers": {Type: schema.FieldTypeRefArray, Target: "person"},
				},
			},
		},
	}
	lines := strings.Split(content, "\n")

	tests := []struct {
		name       string
		line       int
		needle     string
		wantTarget string
		want       bool
	}{
		{name: "quoted scalar", line: 3, needle: "people/freya", wantTarget: "people/freya", want: true},
		{name: "block array bare item", line: 5, needle: "people/loki", wantTarget: "people/loki", want: true},
		{name: "block array quoted item", line: 6, needle: "people/freya", wantTarget: "people/freya", want: true},
		{name: "inline array second item", line: 7, needle: "people/loki", wantTarget: "people/loki", want: true},
		{name: "non-ref string", line: 2, needle: "people/freya", want: false},
		{name: "field key", line: 3, needle: "owner", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			column := strings.Index(lines[tt.line], tt.needle)
			if column < 0 {
				t.Fatalf("needle %q not found on line %d", tt.needle, tt.line)
			}
			ref, ok := SchemaFieldRefAtPosition(doc, sch, tt.line, column+1)
			if ok != tt.want {
				t.Fatalf("ok = %v, want %v (ref = %#v)", ok, tt.want, ref)
			}
			if ok && ref.TargetRaw != tt.wantTarget {
				t.Errorf("target = %q, want %q", ref.TargetRaw, tt.wantTarget)
			}
		})
	}
}
