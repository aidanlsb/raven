package importsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestRun_SlugifiesImportMatchValueAsSinglePathComponent(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
traits: {}
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type: "person",
		},
		Items: []map[string]interface{}{
			{"name": "AC/DC #1"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(result.Results))
	}
	if got := result.Results[0].Action; got != "created" {
		t.Fatalf("action = %q, want created: %#v", got, result.Results[0])
	}
	if got, want := result.Results[0].File, "people/ac-dc-1.md"; got != want {
		t.Fatalf("file = %q, want %q", got, want)
	}

	v.AssertFileExists("people/ac-dc-1.md")
	v.AssertFileNotExists("people/ac/dc#1.md")
	v.AssertFileContains("people/ac-dc-1.md", "AC/DC #1")
}

func TestRun_UpdatesExistingObjectUsingSlugifiedImportMatchValue(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  person:
    default_path: people/
    name_field: name
    fields:
      name:
        type: string
        required: true
      email:
        type: string
traits: {}
`).
		WithFile("people/ac-dc-1.md", `---
type: person
name: AC/DC #1
---
# AC/DC #1
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type: "person",
		},
		Items: []map[string]interface{}{
			{
				"name":  "AC/DC #1",
				"email": "band@example.com",
			},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if len(result.Results) != 1 {
		t.Fatalf("results len = %d, want 1", len(result.Results))
	}
	if got := result.Results[0].Action; got != "updated" {
		t.Fatalf("action = %q, want updated: %#v", got, result.Results[0])
	}

	v.AssertFileContains("people/ac-dc-1.md", "email: band@example.com")
	v.AssertFileNotExists("people/ac/dc#1.md")
}
