package schemasvc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/schemadoc"
)

func TestBuildTypeRenamePlanOnlyTransformsSchema(t *testing.T) {
	t.Parallel()

	schemaYAML := []byte(`version: 2
types:
  event:
    default_path: events/
    description: Old
    fields:
      title: { type: string }
  project:
    fields:
      kickoff:
        type: ref
        target: event
traits: {}
`)

	plan, err := BuildTypeRenamePlan(TypeRenamePlanRequest{
		SchemaDoc:      loadRenameDocument(t, schemaYAML),
		OldName:        "event",
		NewName:        "meeting",
		Description:    "New",
		OldDefaultPath: "events/",
	})
	if err != nil {
		t.Fatalf("BuildTypeRenamePlan: %v", err)
	}

	core := string(plan.SchemaYAML)
	for _, expected := range []string{"meeting:", "description: New", "target: meeting", "default_path: events/"} {
		if !strings.Contains(core, expected) {
			t.Fatalf("core schema missing %q:\n%s", expected, core)
		}
	}
	if strings.Contains(core, "event:") {
		t.Fatalf("core schema still contains old type key:\n%s", core)
	}
	if got := plan.CoreSchemaMutations; got != 3 {
		t.Fatalf("CoreSchemaMutations = %d, want 3", got)
	}
	if plan.DefaultPathOld != "events/" || plan.DefaultPathNew != "meetings/" {
		t.Fatalf("default path plan = %q → %q, want events/ → meetings/", plan.DefaultPathOld, plan.DefaultPathNew)
	}
	if !strings.Contains(string(plan.SchemaYAMLWithDefaultPath), "default_path: meetings/") {
		t.Fatalf("optional schema does not contain renamed default path:\n%s", plan.SchemaYAMLWithDefaultPath)
	}
}

func TestBuildFieldRenamePlanReturnsTemplateSpec(t *testing.T) {
	t.Parallel()

	schemaYAML := []byte(`version: 2
types:
  person:
    name_field: name
    template: templates/person.md
    fields:
      name: { type: string }
traits: {}
`)

	plan, err := BuildFieldRenamePlan(FieldRenamePlanRequest{
		SchemaDoc: loadRenameDocument(t, schemaYAML),
		TypeName:  "person",
		OldField:  "name",
		NewField:  "full_name",
	})
	if err != nil {
		t.Fatalf("BuildFieldRenamePlan: %v", err)
	}

	output := string(plan.SchemaYAML)
	if !strings.Contains(output, "full_name:") || !strings.Contains(output, "name_field: full_name") {
		t.Fatalf("schema did not rename field and name_field:\n%s", output)
	}
	if plan.TemplateSpec != "templates/person.md" {
		t.Fatalf("TemplateSpec = %q, want templates/person.md", plan.TemplateSpec)
	}
	if len(plan.Changes) != 2 {
		t.Fatalf("len(Changes) = %d, want 2", len(plan.Changes))
	}
}

func loadRenameDocument(t *testing.T, schemaYAML []byte) *schemadoc.Document {
	t.Helper()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), schemaYAML, 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}
	doc, err := schemadoc.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema document: %v", err)
	}
	return doc
}
