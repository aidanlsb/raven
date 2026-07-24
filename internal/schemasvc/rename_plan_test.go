package schemasvc

import (
	"errors"
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

func TestBuildTypeRenamePlanDescriptionMutations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                  string
		description           string
		wantDescription       string
		wantDescriptionChange bool
	}{
		{
			name:                  "updates description",
			description:           "New description",
			wantDescription:       "description: New description",
			wantDescriptionChange: true,
		},
		{
			name:                  "clears description",
			description:           "-",
			wantDescription:       "",
			wantDescriptionChange: true,
		},
		{
			name:                  "same description is unchanged",
			description:           "Old description",
			wantDescription:       "description: Old description",
			wantDescriptionChange: false,
		},
		{
			name:                  "empty description leaves existing value",
			description:           "",
			wantDescription:       "description: Old description",
			wantDescriptionChange: false,
		},
	}

	const schemaYAML = `version: 2
types:
  event:
    description: Old description
    default_path: custom/
    fields:
      title: { type: string }
traits: {}
`
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan, err := BuildTypeRenamePlan(TypeRenamePlanRequest{
				SchemaDoc:      loadRenameDocument(t, []byte(schemaYAML)),
				OldName:        "event",
				NewName:        "meeting",
				Description:    tt.description,
				OldDefaultPath: "custom/",
			})
			if err != nil {
				t.Fatalf("BuildTypeRenamePlan() error = %v", err)
			}

			output := string(plan.SchemaYAML)
			if tt.wantDescription == "" {
				if strings.Contains(output, "description:") {
					t.Fatalf("schema retained cleared description:\n%s", output)
				}
			} else if !strings.Contains(output, tt.wantDescription) {
				t.Fatalf("schema missing %q:\n%s", tt.wantDescription, output)
			}
			gotDescriptionChanges := 0
			for _, change := range plan.Changes {
				if change.ChangeType == "schema_description" {
					gotDescriptionChanges++
				}
			}
			wantDescriptionChanges := 0
			if tt.wantDescriptionChange {
				wantDescriptionChanges = 1
			}
			if gotDescriptionChanges != wantDescriptionChanges {
				t.Fatalf("description changes = %d, want %d: %#v", gotDescriptionChanges, wantDescriptionChanges, plan.Changes)
			}
			if plan.DefaultPathMutation || len(plan.OptionalChanges) != 0 || plan.SchemaYAMLWithDefaultPath != nil {
				t.Fatalf("custom default path unexpectedly produced rename option: %#v", plan)
			}
		})
	}
}

func TestBuildTypeRenamePlanErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		doc  func(*testing.T) *schemadoc.Document
		code ErrorCode
	}{
		{
			name: "nil document",
			doc:  func(*testing.T) *schemadoc.Document { return nil },
			code: ErrorInternal,
		},
		{
			name: "missing types section",
			doc: func(t *testing.T) *schemadoc.Document {
				return loadRenameDocument(t, []byte("version: 2\ntraits: {}\n"))
			},
			code: ErrorSchemaInvalid,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := BuildTypeRenamePlan(TypeRenamePlanRequest{
				SchemaDoc: tt.doc(t),
				OldName:   "event",
				NewName:   "meeting",
			})
			requireRenamePlanCode(t, err, tt.code)
		})
	}
}

func TestBuildFieldRenamePlanValidation(t *testing.T) {
	t.Parallel()

	const baseSchema = `version: 2
types:
  person:
    name_field: name
    fields:
      name: { type: string }
      email: { type: string }
traits: {}
`
	tests := []struct {
		name         string
		doc          func(*testing.T) *schemadoc.Document
		typeName     string
		oldField     string
		newField     string
		wantCode     ErrorCode
		wantChanges  int
		wantContains []string
	}{
		{
			name:         "renames name field",
			doc:          func(t *testing.T) *schemadoc.Document { return loadRenameDocument(t, []byte(baseSchema)) },
			typeName:     "person",
			oldField:     "name",
			newField:     "full_name",
			wantChanges:  2,
			wantContains: []string{"full_name:", "name_field: full_name"},
		},
		{
			name:         "renames ordinary field",
			doc:          func(t *testing.T) *schemadoc.Document { return loadRenameDocument(t, []byte(baseSchema)) },
			typeName:     "person",
			oldField:     "email",
			newField:     "contact",
			wantChanges:  1,
			wantContains: []string{"contact:", "name_field: name"},
		},
		{
			name:     "nil document",
			doc:      func(*testing.T) *schemadoc.Document { return nil },
			typeName: "person",
			oldField: "name",
			newField: "full_name",
			wantCode: ErrorInternal,
		},
		{
			name: "missing types section",
			doc: func(t *testing.T) *schemadoc.Document {
				return loadRenameDocument(t, []byte("version: 2\ntraits: {}\n"))
			},
			typeName: "person",
			oldField: "name",
			newField: "full_name",
			wantCode: ErrorSchemaInvalid,
		},
		{
			name:     "missing type",
			doc:      func(t *testing.T) *schemadoc.Document { return loadRenameDocument(t, []byte(baseSchema)) },
			typeName: "company",
			oldField: "name",
			newField: "full_name",
			wantCode: ErrorTypeNotFound,
		},
		{
			name: "type has no fields",
			doc: func(t *testing.T) *schemadoc.Document {
				return loadRenameDocument(t, []byte("version: 2\ntypes:\n  person:\n    description: Person\ntraits: {}\n"))
			},
			typeName: "person",
			oldField: "name",
			newField: "full_name",
			wantCode: ErrorFieldNotFound,
		},
		{
			name:     "old field missing",
			doc:      func(t *testing.T) *schemadoc.Document { return loadRenameDocument(t, []byte(baseSchema)) },
			typeName: "person",
			oldField: "missing",
			newField: "full_name",
			wantCode: ErrorFieldNotFound,
		},
		{
			name:     "new field already exists",
			doc:      func(t *testing.T) *schemadoc.Document { return loadRenameDocument(t, []byte(baseSchema)) },
			typeName: "person",
			oldField: "name",
			newField: "email",
			wantCode: ErrorObjectExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			plan, err := BuildFieldRenamePlan(FieldRenamePlanRequest{
				SchemaDoc: tt.doc(t),
				TypeName:  tt.typeName,
				OldField:  tt.oldField,
				NewField:  tt.newField,
			})
			if tt.wantCode != "" {
				requireRenamePlanCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("BuildFieldRenamePlan() error = %v", err)
			}
			if len(plan.Changes) != tt.wantChanges {
				t.Fatalf("len(Changes) = %d, want %d: %#v", len(plan.Changes), tt.wantChanges, plan.Changes)
			}
			output := string(plan.SchemaYAML)
			for _, value := range tt.wantContains {
				if !strings.Contains(output, value) {
					t.Fatalf("schema missing %q:\n%s", value, output)
				}
			}
			if strings.Contains(output, "\n      "+tt.oldField+":") {
				t.Fatalf("schema retained old field %q:\n%s", tt.oldField, output)
			}
		})
	}
}

func TestSuggestRenamedDefaultPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		oldPath string
		oldName string
		newName string
		want    string
		wantOK  bool
	}{
		{name: "singular", oldPath: "event/", oldName: "event", newName: "meeting", want: "meeting/", wantOK: true},
		{name: "plural", oldPath: "events", oldName: "event", newName: "meeting", want: "meetings/", wantOK: true},
		{name: "nested plural", oldPath: "objects/events/", oldName: "event", newName: "meeting", want: "objects/meetings/", wantOK: true},
		{name: "unrelated directory", oldPath: "calendar/", oldName: "event", newName: "meeting"},
		{name: "empty path", oldPath: "", oldName: "event", newName: "meeting"},
		{name: "root path", oldPath: "/", oldName: "event", newName: "meeting"},
		{name: "unchanged name", oldPath: "event/", oldName: "event", newName: "event"},
	}

	for _, tt := range tests {
		got, ok := suggestRenamedDefaultPath(tt.oldPath, tt.oldName, tt.newName)
		if got != tt.want || ok != tt.wantOK {
			t.Errorf("%s: suggestRenamedDefaultPath(%q, %q, %q) = (%q, %v), want (%q, %v)",
				tt.name, tt.oldPath, tt.oldName, tt.newName, got, ok, tt.want, tt.wantOK)
		}
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

func requireRenamePlanCode(t *testing.T, err error, want ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	var svcErr *Error
	if !errors.As(err, &svcErr) {
		t.Fatalf("error = %T %v, want schemasvc error", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %s, want %s", svcErr.Code, want)
	}
}
