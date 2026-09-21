package schemasvc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func schemaTestRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{})
}

func requireSchemaCode(t *testing.T, err error, want codes.ErrorCode) {
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

func TestAddType(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		schema        string
		req           AddTypeRequest
		wantCode      codes.ErrorCode
		wantPath      string
		wantNameField string
	}{
		{
			name:     "defaults path to type name",
			schema:   testutil.MinimalSchema(),
			req:      AddTypeRequest{TypeName: "meeting"},
			wantPath: "meeting/",
		},
		{
			name:          "auto-creates name field",
			schema:        testutil.MinimalSchema(),
			req:           AddTypeRequest{TypeName: "company", NameField: "name", Description: "Orgs"},
			wantPath:      "company/",
			wantNameField: "name",
		},
		{
			name:     "empty name",
			schema:   testutil.MinimalSchema(),
			req:      AddTypeRequest{},
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:     "builtin type",
			schema:   testutil.MinimalSchema(),
			req:      AddTypeRequest{TypeName: "page"},
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:     "already exists",
			schema:   testutil.PersonProjectSchema(),
			req:      AddTypeRequest{TypeName: "person"},
			wantCode: codes.ErrObjectExists,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vault := testutil.NewTestVault(t).WithSchema(tt.schema).Build()
			result, err := AddType(schemaTestRuntime(t, vault.Path), tt.req)
			if tt.wantCode != "" {
				requireSchemaCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("AddType() error = %v", err)
			}
			if result.DefaultPath != tt.wantPath {
				t.Fatalf("DefaultPath = %q, want %q", result.DefaultPath, tt.wantPath)
			}
			if result.NameField != tt.wantNameField {
				t.Fatalf("NameField = %q, want %q", result.NameField, tt.wantNameField)
			}
			loaded, err := schema.Load(vault.Path)
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}
			typeDef, ok := loaded.Types[tt.req.TypeName]
			if !ok {
				t.Fatalf("type %q not in schema", tt.req.TypeName)
			}
			if typeDef.DefaultPath != tt.wantPath {
				t.Fatalf("schema default_path = %q, want %q", typeDef.DefaultPath, tt.wantPath)
			}
			if tt.wantNameField != "" {
				if typeDef.NameField != tt.wantNameField {
					t.Fatalf("schema name_field = %q, want %q", typeDef.NameField, tt.wantNameField)
				}
				field := typeDef.Fields[tt.wantNameField]
				if field == nil || field.Type != schema.FieldTypeString || !field.Required {
					t.Fatalf("auto-created field = %#v, want required string", field)
				}
			}
		})
	}
}

func TestAddField(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		req      AddFieldRequest
		wantCode codes.ErrorCode
		wantType string
		wantMsg  string
	}{
		{
			name:     "adds string field",
			req:      AddFieldRequest{TypeName: "project", FieldName: "summary", FieldType: "string"},
			wantType: "string",
		},
		{
			name:     "adds ref field",
			req:      AddFieldRequest{TypeName: "project", FieldName: "sponsor", FieldType: "ref", Target: "person"},
			wantType: "ref",
		},
		{
			name:     "empty names",
			req:      AddFieldRequest{},
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:     "builtin type",
			req:      AddFieldRequest{TypeName: "page", FieldName: "extra", FieldType: "string"},
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:     "missing type",
			req:      AddFieldRequest{TypeName: "company", FieldName: "name", FieldType: "string"},
			wantCode: codes.ErrTypeNotFound,
		},
		{
			name:     "existing field",
			req:      AddFieldRequest{TypeName: "project", FieldName: "title", FieldType: "string"},
			wantCode: codes.ErrObjectExists,
		},
		{
			name:     "schema type name",
			req:      AddFieldRequest{TypeName: "project", FieldName: "lead", FieldType: "person"},
			wantCode: codes.ErrInvalidInput,
			wantMsg:  "--type ref --target person",
		},
		{
			name:     "ref without target",
			req:      AddFieldRequest{TypeName: "project", FieldName: "lead", FieldType: "ref"},
			wantCode: codes.ErrInvalidInput,
			wantMsg:  "--target",
		},
		{
			name:     "enum without values",
			req:      AddFieldRequest{TypeName: "project", FieldName: "priority", FieldType: "enum"},
			wantCode: codes.ErrInvalidInput,
			wantMsg:  "--values",
		},
		{
			name:     "target on non-ref",
			req:      AddFieldRequest{TypeName: "project", FieldName: "summary", FieldType: "string", Target: "person"},
			wantCode: codes.ErrInvalidInput,
			wantMsg:  "--target is only valid for ref fields",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
			result, err := AddField(schemaTestRuntime(t, vault.Path), tt.req)
			if tt.wantCode != "" {
				requireSchemaCode(t, err, tt.wantCode)
				if tt.wantMsg != "" {
					var svcErr *Error
					if !errors.As(err, &svcErr) {
						t.Fatalf("error = %T, want schemasvc error", err)
					}
					if !strings.Contains(svcErr.Message, tt.wantMsg) && !strings.Contains(svcErr.Suggestion, tt.wantMsg) {
						t.Fatalf("error %q suggestion %q does not contain %q", svcErr.Message, svcErr.Suggestion, tt.wantMsg)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("AddField() error = %v", err)
			}
			if result.FieldType != tt.wantType {
				t.Fatalf("FieldType = %q, want %q", result.FieldType, tt.wantType)
			}
			loaded, err := schema.Load(vault.Path)
			if err != nil {
				t.Fatalf("load schema: %v", err)
			}
			field := loaded.Types[tt.req.TypeName].Fields[tt.req.FieldName]
			if field == nil {
				t.Fatalf("field %s.%s missing", tt.req.TypeName, tt.req.FieldName)
			}
			if string(field.Type) != tt.wantType {
				t.Fatalf("schema field type = %q, want %q", field.Type, tt.wantType)
			}
		})
	}
}

func TestAddTrait_TrimsValuesAndCoercesBooleanDefault(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	rt := schemaTestRuntime(t, vaultPath)
	_, err := AddTrait(rt, AddTraitRequest{
		TraitName: "priority",
		TraitType: "boolean",
		Values:    "low, medium, ,high",
		Default:   "true",
	})
	if err != nil {
		t.Fatalf("AddTrait failed: %v", err)
	}
	if _, ok := rt.Schema.Traits["priority"]; !ok {
		t.Fatal("runtime schema was not refreshed after AddTrait")
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	trait, ok := loaded.Traits["priority"]
	if !ok {
		t.Fatalf("expected trait priority to exist")
	}

	if got, want := trait.Type, schema.FieldType("boolean"); got != want {
		t.Fatalf("trait type = %q, want %q", got, want)
	}

	wantValues := []string{"low", "medium", "high"}
	if len(trait.Values) != len(wantValues) {
		t.Fatalf("trait values len = %d, want %d (%v)", len(trait.Values), len(wantValues), trait.Values)
	}
	for i := range wantValues {
		if trait.Values[i] != wantValues[i] {
			t.Fatalf("trait value[%d] = %q, want %q", i, trait.Values[i], wantValues[i])
		}
	}

	if got, ok := trait.Default.(bool); !ok || !got {
		t.Fatalf("trait default = %#v, want bool(true)", trait.Default)
	}
}

func TestAddTrait_PreservesStringDefaultForNonBooleanTypes(t *testing.T) {
	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "schema.yaml"), []byte("version: 2\n"), 0o644); err != nil {
		t.Fatalf("write schema.yaml: %v", err)
	}

	_, err := AddTrait(schemaTestRuntime(t, vaultPath), AddTraitRequest{
		TraitName: "status",
		TraitType: "enum",
		Values:    "todo,doing,done",
		Default:   "doing",
	})
	if err != nil {
		t.Fatalf("AddTrait failed: %v", err)
	}

	loaded, err := schema.Load(vaultPath)
	if err != nil {
		t.Fatalf("load schema: %v", err)
	}

	trait, ok := loaded.Traits["status"]
	if !ok {
		t.Fatalf("expected trait status to exist")
	}
	if got, ok := trait.Default.(string); !ok || got != "doing" {
		t.Fatalf("trait default = %#v, want string(\"doing\")", trait.Default)
	}
}
