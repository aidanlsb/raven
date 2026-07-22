package importsvc

import (
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestBuildMappingConfigReturnsSharedServiceError(t *testing.T) {
	t.Parallel()

	_, err := BuildMappingConfig(BuildMappingConfigRequest{})
	svcErr, ok := svcerr.AsError(err)
	if !ok || svcErr.Code != codes.ErrInvalidInput {
		t.Fatalf("BuildMappingConfig() error = %#v, want %s", svcErr, codes.ErrInvalidInput)
	}
}

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

func TestRun_ContentFieldPopulatesBodyOnCreate(t *testing.T) {
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
			Type:         "person",
			ContentField: "bio",
		},
		Items: []map[string]interface{}{
			{"name": "Freya", "bio": "Project lead and architect."},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Results[0].Action; got != "created" {
		t.Fatalf("action = %q, want created: %#v", got, result.Results[0])
	}

	v.AssertFileContains("people/freya.md", "name: Freya")
	v.AssertFileContains("people/freya.md", "Project lead and architect.")
	// The content field is body content, not frontmatter.
	v.AssertFileNotContains("people/freya.md", "bio:")
}

func TestRun_ContentFieldReplacesBodyOnUpdate(t *testing.T) {
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
		WithFile("people/freya.md", `---
type: person
name: Freya
---

# Old body content
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type:         "person",
			ContentField: "bio",
		},
		Items: []map[string]interface{}{
			{"name": "Freya", "bio": "Fresh imported body."},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Results[0].Action; got != "updated" {
		t.Fatalf("action = %q, want updated: %#v", got, result.Results[0])
	}

	v.AssertFileContains("people/freya.md", "Fresh imported body.")
	v.AssertFileNotContains("people/freya.md", "Old body content")
}

// TestRun_RespectsProtectedPathsOnCreate verifies that import refuses to create
// objects in protected paths, matching objectsvc's content-mutation guardrails.
func TestRun_RespectsProtectedPathsOnCreate(t *testing.T) {
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
		WithRavenYAML(`protected_prefixes:
  - people/
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type: "person",
		},
		Items: []map[string]interface{}{
			{"name": "Freya"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Results[0].Action; got != "error" {
		t.Fatalf("action = %q, want error: %#v", got, result.Results[0])
	}
	if got := result.Results[0].Code; got != string(codes.ErrValidationFailed) {
		t.Fatalf("code = %q, want %q: %#v", got, codes.ErrValidationFailed, result.Results[0])
	}
	v.AssertFileNotExists("people/freya.md")
}

// TestRun_RespectsProtectedPathsOnUpdate verifies that import refuses to modify
// an existing object in a protected path. Previously the update flow wrote
// directly, bypassing the protected-path safeguards enforced elsewhere.
func TestRun_RespectsProtectedPathsOnUpdate(t *testing.T) {
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
		WithRavenYAML(`protected_prefixes:
  - people/
`).
		WithFile("people/freya.md", `---
type: person
name: Freya
---

# Freya
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type: "person",
		},
		Items: []map[string]interface{}{
			{"name": "Freya", "email": "freya@example.com"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Results[0].Action; got != "error" {
		t.Fatalf("action = %q, want error: %#v", got, result.Results[0])
	}
	if got := result.Results[0].Code; got != string(codes.ErrValidationFailed) {
		t.Fatalf("code = %q, want %q: %#v", got, codes.ErrValidationFailed, result.Results[0])
	}
	// The protected object must be left untouched.
	v.AssertFileNotContains("people/freya.md", "email: freya@example.com")
}

// TestRun_MissingRequiredFieldOnCreateReportsStructuredError verifies that
// creating an object without a required field surfaces objectsvc's structured
// required-field error per item, rather than silently writing an invalid file.
func TestRun_MissingRequiredFieldOnCreateReportsStructuredError(t *testing.T) {
	t.Parallel()

	v := testutil.NewTestVault(t).
		WithSchema(`version: 1
types:
  task:
    default_path: tasks/
    name_field: name
    fields:
      name:
        type: string
        required: true
      status:
        type: string
        required: true
traits: {}
`).
		Build()

	result, err := Run(RunRequest{
		VaultPath: v.Path,
		MappingConfig: &MappingConfig{
			Type: "task",
		},
		Items: []map[string]interface{}{
			{"name": "Write tests"},
		},
	})
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if got := result.Results[0].Action; got != "error" {
		t.Fatalf("action = %q, want error: %#v", got, result.Results[0])
	}
	if got := result.Results[0].Code; got != string(codes.ErrRequiredFieldMissing) {
		t.Fatalf("code = %q, want %q: %#v", got, codes.ErrRequiredFieldMissing, result.Results[0])
	}
	v.AssertFileNotExists("tasks/write-tests.md")
}
