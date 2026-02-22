package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestSchemaAddTypeAndFieldDescriptions(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	v.RunCLI(
		"schema", "add", "type", "company",
		"--default-path", "companies/",
		"--description", "Companies and organizations",
	).MustSucceed(t)

	v.RunCLI(
		"schema", "add", "field", "company", "website",
		"--type", "string",
		"--description", "Primary website URL",
	).MustSucceed(t)

	v.AssertFileContains("schema.yaml", "description: Companies and organizations")
	v.AssertFileContains("schema.yaml", "description: Primary website URL")

	result := v.RunCLI("schema", "type", "company")
	result.MustSucceed(t)

	typeData, ok := result.Data["type"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected type object in response, got: %#v", result.Data["type"])
	}
	if got := typeData["description"]; got != "Companies and organizations" {
		t.Fatalf("expected type description %q, got %#v", "Companies and organizations", got)
	}

	fields, ok := typeData["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map in response, got: %#v", typeData["fields"])
	}
	website, ok := fields["website"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected website field object in response, got: %#v", fields["website"])
	}
	if got := website["description"]; got != "Primary website URL" {
		t.Fatalf("expected field description %q, got %#v", "Primary website URL", got)
	}
}

func TestSchemaAddTypeDefaultsPathToTypeName(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.MinimalSchema()).
		Build()

	result := v.RunCLI("schema", "add", "type", "meeting")
	result.MustSucceed(t)

	if got := result.Data["default_path"]; got != "meeting/" {
		t.Fatalf("expected default_path %q, got %#v", "meeting/", got)
	}

	v.AssertFileContains("schema.yaml", "meeting:")
	v.AssertFileContains("schema.yaml", "default_path: meeting/")
}

func TestSchemaUpdateAndRemoveTypeFieldDescriptions(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  person:
    default_path: people/
    fields:
      name:
        type: string
        required: true
      email:
        type: string
traits: {}
`).
		Build()

	v.RunCLI(
		"schema", "update", "type", "person",
		"--description", "People and contacts",
	).MustSucceed(t)

	v.RunCLI(
		"schema", "update", "field", "person", "email",
		"--description", "Primary contact email",
	).MustSucceed(t)

	result := v.RunCLI("schema", "type", "person")
	result.MustSucceed(t)

	typeData, ok := result.Data["type"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected type object in response, got: %#v", result.Data["type"])
	}
	if got := typeData["description"]; got != "People and contacts" {
		t.Fatalf("expected type description %q, got %#v", "People and contacts", got)
	}

	fields, ok := typeData["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map in response, got: %#v", typeData["fields"])
	}
	email, ok := fields["email"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected email field object in response, got: %#v", fields["email"])
	}
	if got := email["description"]; got != "Primary contact email" {
		t.Fatalf("expected field description %q, got %#v", "Primary contact email", got)
	}

	v.RunCLI(
		"schema", "update", "field", "person", "email",
		"--description", "-",
	).MustSucceed(t)
	v.RunCLI(
		"schema", "update", "type", "person",
		"--description", "-",
	).MustSucceed(t)

	result = v.RunCLI("schema", "type", "person")
	result.MustSucceed(t)

	typeData, ok = result.Data["type"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected type object in response, got: %#v", result.Data["type"])
	}
	if _, exists := typeData["description"]; exists {
		t.Fatalf("expected type description to be removed, got: %#v", typeData["description"])
	}

	fields, ok = typeData["fields"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected fields map in response, got: %#v", typeData["fields"])
	}
	email, ok = fields["email"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected email field object in response, got: %#v", fields["email"])
	}
	if _, exists := email["description"]; exists {
		t.Fatalf("expected field description to be removed, got: %#v", email["description"])
	}
}
