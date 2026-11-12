//go:build integration

package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestIntegration_RQLRelationshipCoverage(t *testing.T) {
	t.Parallel()
	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  company:
    default_path: company/
    name_field: name
    fields:
      name:
        type: string
        required: true
      tier:
        type: number
  project:
    default_path: project/
    name_field: name
    fields:
      name:
        type: string
        required: true
      status:
        type: string
      company:
        type: ref
        target: company
  person:
    default_path: person/
    name_field: name
    fields:
      name:
        type: string
        required: true
      team:
        type: string
  task:
    default_path: task/
    name_field: title
    fields:
      title:
        type: string
        required: true
      status:
        type: string
      project:
        type: ref
        target: project
      owner:
        type: ref
        target: person
traits:
  related:
    type: ref
`).
		WithFile("company/acme.md", `---
type: company
name: Acme
tier: 1
---
`).
		WithFile("project/web.md", `---
type: project
name: Website
status: active
company: company/acme
---
`).
		WithFile("person/ada.md", `---
type: person
name: Ada
team: infra
---

@related(person/bob)
`).
		WithFile("person/bob.md", `---
type: person
name: Bob
team: platform
---
`).
		WithFile("task/one.md", `---
type: task
title: Ship
status: open
project: project/web
owner: person/ada
---
`).
		Build()

	forward := v.RunCLI("query", "type:task .project.status==active")
	forward.MustSucceed(t)
	forward.AssertResultCount(t, "items", 1)

	twoHop := v.RunCLI("query", "type:task .project.company.tier==1")
	twoHop.MustSucceed(t)
	twoHop.AssertResultCount(t, "items", 1)

	reverse := v.RunCLI("query", "type:person refd(type:task .owner==this .status==open)")
	reverse.MustSucceed(t)
	reverse.AssertResultCount(t, "items", 1)

	flatten := v.RunCLI("query", "type:company refd(type:task .project.company==this)")
	flatten.MustSucceed(t)
	flatten.AssertResultCount(t, "items", 1)

	nested := v.RunCLI("query", "type:task refs(type:project refs(type:company .tier==1))")
	nested.MustSucceed(t)
	nested.AssertResultCount(t, "items", 1)

	traitRef := v.RunCLI("query", "type:person refd(trait:related)")
	traitRef.MustSucceed(t)
	traitRef.AssertResultCount(t, "items", 1)

	idCmp := v.RunCLI("query", "type:task .project==[[project/web]]")
	idCmp.MustSucceed(t)
	idCmp.AssertResultCount(t, "items", 1)

	invalid := v.RunCLI("query", "type:task .status.active==yes")
	if invalid.OK {
		t.Fatal("expected invalid path to fail")
	}

	topThis := v.RunCLI("query", "type:task .owner==this")
	if topThis.OK {
		t.Fatal("expected top-level this to fail")
	}
}
