package cli_test

import (
	"testing"

	"github.com/aidanlsb/raven/internal/testutil"
)

func TestSchemaRenameField_PreviewDoesNotModifyFiles(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  person:
    default_path: people/
    name_field: name
    template: templates/person.md
    fields:
      name: { type: string, required: true }
      email: { type: string }
traits: {}
`).
		WithRavenYAML(`daily_directory: daily
queries:
  person-by-email:
    query: 'object:person .email=="alice@example.com"'
  overdue:
    query: 'trait:due .value==past'
`).
		WithFile("templates/person.md", "Email: {{field.email}}\n").
		WithFile("people/alice.md", `---
type: person
name: Alice
email: alice@example.com
---
# Alice
`).
		WithFile("notes/embedded.md", `---
type: page
---
# Alice (embedded)
::person(email="alice@example.com")
`).
		Build()

	// Snapshot contents
	beforeSchema := v.ReadFile("schema.yaml")
	beforeRaven := v.ReadFile("raven.yaml")
	beforeTemplate := v.ReadFile("templates/person.md")
	beforePerson := v.ReadFile("people/alice.md")
	beforeEmbedded := v.ReadFile("notes/embedded.md")

	// Preview (no --confirm)
	res := v.RunCLI("schema", "rename", "field", "person", "email", "email_address")
	res.MustSucceed(t)

	// Should report preview
	if res.Data == nil || res.Data["preview"] != true {
		t.Fatalf("expected preview=true in response, got: %v\nRaw: %s", res.Data, res.RawJSON)
	}

	// No files should change
	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatalf("expected schema.yaml unchanged in preview mode")
	}
	if got := v.ReadFile("raven.yaml"); got != beforeRaven {
		t.Fatalf("expected raven.yaml unchanged in preview mode")
	}
	if got := v.ReadFile("templates/person.md"); got != beforeTemplate {
		t.Fatalf("expected template file unchanged in preview mode")
	}
	if got := v.ReadFile("people/alice.md"); got != beforePerson {
		t.Fatalf("expected person file unchanged in preview mode")
	}
	if got := v.ReadFile("notes/embedded.md"); got != beforeEmbedded {
		t.Fatalf("expected embedded file unchanged in preview mode")
	}
}

func TestSchemaRenameField_ConfirmUpdatesSchemaTemplatesQueriesAndFiles(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(`version: 2
types:
  person:
    default_path: people/
    name_field: name
    template: templates/person.md
    fields:
      name: { type: string, required: true }
      email: { type: string }
traits: {}
`).
		WithRavenYAML(`daily_directory: daily
queries:
  person-by-email:
    query: 'object:person .email=="alice@example.com"'
  overdue:
    query: 'trait:due .value==past'
`).
		WithFile("templates/person.md", "Email: {{field.email}}\n").
		WithFile("people/alice.md", `---
type: person
name: Alice
email: alice@example.com
---
# Alice
`).
		WithFile("notes/embedded.md", `---
type: page
---
# Alice (embedded)
::person(email="alice@example.com")
`).
		Build()

	res := v.RunCLI("schema", "rename", "field", "person", "email", "email_address", "--confirm")
	res.MustSucceed(t)

	// schema.yaml field key
	v.AssertFileContains("schema.yaml", "email_address:")
	v.AssertFileNotContains("schema.yaml", "\n      email:")

	// template file updated
	v.AssertFileContains("templates/person.md", "{{field.email_address}}")
	v.AssertFileNotContains("templates/person.md", "{{field.email}}")

	// raven.yaml saved query updated (only object:person query)
	v.AssertFileContains("raven.yaml", `.email_address==`)
	v.AssertFileNotContains("raven.yaml", `.email==`)
	v.AssertFileContains("raven.yaml", "trait:due")

	// frontmatter updated
	v.AssertFileContains("people/alice.md", "email_address: alice@example.com")
	v.AssertFileNotContains("people/alice.md", "\nemail: ")

	// embedded ::type(...) updated
	// Serialization may omit quotes if not required.
	v.AssertFileContains("notes/embedded.md", `::person(email_address=alice@example.com)`)
	v.AssertFileNotContains("notes/embedded.md", `::person(email="alice@example.com")`)
}

func TestSchemaRenameField_ConflictInFrontmatterBlocksRename(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("people/alice.md", `---
type: person
name: Alice
email: alice@example.com
email_address: alice@new.example.com
---
# Alice
`).
		Build()

	beforeSchema := v.ReadFile("schema.yaml")
	beforePerson := v.ReadFile("people/alice.md")

	res := v.RunCLI("schema", "rename", "field", "person", "email", "email_address")
	res.MustFail(t, "DATA_INTEGRITY_BLOCK")

	// Should not have applied anything
	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatalf("expected schema.yaml unchanged when conflicts exist")
	}
	if got := v.ReadFile("people/alice.md"); got != beforePerson {
		t.Fatalf("expected file unchanged when conflicts exist")
	}
}

func TestSchemaRenameField_ConflictInEmbeddedDeclBlocksRename(t *testing.T) {
	v := testutil.NewTestVault(t).
		WithSchema(testutil.PersonProjectSchema()).
		WithFile("notes/embedded.md", `---
type: page
---
# Alice
::person(email="alice@example.com", email_address="alice@new.example.com")
`).
		Build()

	beforeSchema := v.ReadFile("schema.yaml")
	beforeFile := v.ReadFile("notes/embedded.md")

	res := v.RunCLI("schema", "rename", "field", "person", "email", "email_address")
	res.MustFail(t, "DATA_INTEGRITY_BLOCK")

	if got := v.ReadFile("schema.yaml"); got != beforeSchema {
		t.Fatalf("expected schema.yaml unchanged when conflicts exist")
	}
	if got := v.ReadFile("notes/embedded.md"); got != beforeFile {
		t.Fatalf("expected file unchanged when conflicts exist")
	}
}
