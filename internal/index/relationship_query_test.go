package index

import (
	"sort"
	"testing"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/query"
	"github.com/aidanlsb/raven/internal/schema"
)

func graphSchema() *schema.Schema {
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
		},
	}
	sch.Types["task"] = &schema.TypeDefinition{
		Fields: map[string]*schema.FieldDefinition{
			"status":  {Type: schema.FieldTypeString},
			"project": {Type: schema.FieldTypeRef, Target: "project"},
			"owner":   {Type: schema.FieldTypeRef, Target: "person"},
		},
	}
	sch.Traits["related"] = &schema.TraitDefinition{Type: schema.FieldTypeRef}
	return sch
}

func TestScratchVaultRelationshipQueries(t *testing.T) {
	t.Parallel()

	db, err := OpenInMemory()
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	db.SetAutoResolveRefs(false)
	sch := graphSchema()

	docs := map[string]string{
		"company/acme.md": `---
type: company
name: Acme
tier: 1
---
`,
		"project/web.md": `---
type: project
name: Website
status: active
company: company/acme
---
`,
		"person/ada.md": `---
type: person
name: Ada
team: infra
---

@related(person/bob)
`,
		"person/bob.md": `---
type: person
name: Bob
team: platform
---
`,
		"task/one.md": `---
type: task
status: open
project: project/web
owner: person/ada
---
`,
	}
	for path, content := range docs {
		doc, err := parser.ParseDocument(content, path, "/vault")
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		if err := db.IndexDocument(doc, sch); err != nil {
			t.Fatalf("index %s: %v", path, err)
		}
	}
	if _, err := db.ResolveReferences("daily"); err != nil {
		t.Fatalf("resolve: %v", err)
	}

	exec := query.NewExecutor(db.db)
	exec.SetSchema(sch)

	run := func(qstr string) []string {
		t.Helper()
		q, err := query.Parse(qstr)
		if err != nil {
			t.Fatalf("parse %q: %v", qstr, err)
		}
		if err := query.NewValidator(sch).Validate(q); err != nil {
			t.Fatalf("validate %q: %v", qstr, err)
		}
		result, err := exec.Run(q, query.RunRequest{})
		if err != nil {
			t.Fatalf("run %q: %v", qstr, err)
		}
		ids := make([]string, 0, len(result.Objects))
		for _, obj := range result.Objects {
			ids = append(ids, obj.ID)
		}
		sort.Strings(ids)
		return ids
	}

	if got := run("type:task .project.status==active"); len(got) != 1 || got[0] != "task/one" {
		t.Fatalf("forward path = %v", got)
	}
	if got := run("type:task .project.company.tier==1"); len(got) != 1 || got[0] != "task/one" {
		t.Fatalf("two-hop = %v", got)
	}
	if got := run("type:person refd(type:task .owner==this .status==open)"); len(got) != 1 || got[0] != "person/ada" {
		t.Fatalf("reverse = %v", got)
	}
	if got := run("type:company refd(type:task .project.company==this)"); len(got) != 1 || got[0] != "company/acme" {
		t.Fatalf("flatten = %v", got)
	}
	if got := run("type:task refs(type:project refs(type:company .tier==1))"); len(got) != 1 || got[0] != "task/one" {
		t.Fatalf("nested refs = %v", got)
	}
	if got := run("type:person refd(trait:related)"); len(got) != 1 || got[0] != "person/bob" {
		t.Fatalf("bare trait refd = %v", got)
	}
	if got := run("type:person refd(trait:related .value==this)"); len(got) != 1 || got[0] != "person/bob" {
		t.Fatalf("trait value this = %v", got)
	}
	if got := run("type:task .project==[[project/web]]"); len(got) != 1 || got[0] != "task/one" {
		t.Fatalf("id compare = %v", got)
	}
}
