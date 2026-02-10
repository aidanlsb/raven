package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestApplyFieldMappings(t *testing.T) {
	tests := []struct {
		name     string
		item     map[string]interface{}
		fieldMap map[string]string
		want     map[string]interface{}
	}{
		{
			name:     "no mappings passes through",
			item:     map[string]interface{}{"name": "Freya", "email": "f@a.realm"},
			fieldMap: map[string]string{},
			want:     map[string]interface{}{"name": "Freya", "email": "f@a.realm"},
		},
		{
			name:     "renames mapped fields",
			item:     map[string]interface{}{"full_name": "Freya", "mail": "f@a.realm"},
			fieldMap: map[string]string{"full_name": "name", "mail": "email"},
			want:     map[string]interface{}{"name": "Freya", "email": "f@a.realm"},
		},
		{
			name:     "unmapped fields pass through",
			item:     map[string]interface{}{"full_name": "Freya", "age": float64(30)},
			fieldMap: map[string]string{"full_name": "name"},
			want:     map[string]interface{}{"name": "Freya", "age": float64(30)},
		},
		{
			name:     "nil field map passes through",
			item:     map[string]interface{}{"name": "Thor"},
			fieldMap: nil,
			want:     map[string]interface{}{"name": "Thor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := applyFieldMappings(tt.item, tt.fieldMap)
			if len(got) != len(tt.want) {
				t.Errorf("got %d fields, want %d", len(got), len(tt.want))
			}
			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("key %q: got %v, want %v", k, gotV, wantV)
				}
			}
		})
	}
}

func TestMatchKeyValue(t *testing.T) {
	tests := []struct {
		name     string
		mapped   map[string]interface{}
		matchKey string
		wantVal  string
		wantOK   bool
	}{
		{
			name:     "string value",
			mapped:   map[string]interface{}{"name": "Freya"},
			matchKey: "name",
			wantVal:  "Freya",
			wantOK:   true,
		},
		{
			name:     "missing key",
			mapped:   map[string]interface{}{"email": "f@a.realm"},
			matchKey: "name",
			wantVal:  "",
			wantOK:   false,
		},
		{
			name:     "empty string",
			mapped:   map[string]interface{}{"name": ""},
			matchKey: "name",
			wantVal:  "",
			wantOK:   false,
		},
		{
			name:     "numeric value",
			mapped:   map[string]interface{}{"id": float64(42)},
			matchKey: "id",
			wantVal:  "42",
			wantOK:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotVal, gotOK := matchKeyValue(tt.mapped, tt.matchKey)
			if gotOK != tt.wantOK {
				t.Errorf("ok: got %v, want %v", gotOK, tt.wantOK)
			}
			if gotVal != tt.wantVal {
				t.Errorf("val: got %q, want %q", gotVal, tt.wantVal)
			}
		})
	}
}

func TestFieldsToStringMap(t *testing.T) {
	tests := []struct {
		name   string
		fields map[string]interface{}
		want   map[string]string
	}{
		{
			name:   "string fields",
			fields: map[string]interface{}{"name": "Freya", "email": "f@a.realm"},
			want:   map[string]string{"name": "Freya", "email": "f@a.realm"},
		},
		{
			name:   "integer number",
			fields: map[string]interface{}{"rating": float64(5)},
			want:   map[string]string{"rating": "5"},
		},
		{
			name:   "float number",
			fields: map[string]interface{}{"score": float64(3.14)},
			want:   map[string]string{"score": "3.14"},
		},
		{
			name:   "boolean",
			fields: map[string]interface{}{"active": true},
			want:   map[string]string{"active": "true"},
		},
		{
			name:   "array",
			fields: map[string]interface{}{"tags": []interface{}{"go", "rust"}},
			want:   map[string]string{"tags": "[go, rust]"},
		},
		{
			name:   "nil values skipped",
			fields: map[string]interface{}{"name": "Freya", "notes": nil},
			want:   map[string]string{"name": "Freya"},
		},
		{
			name:   "type field excluded",
			fields: map[string]interface{}{"type": "person", "name": "Freya"},
			want:   map[string]string{"name": "Freya"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := fieldsToStringMap(tt.fields, "person")
			if len(got) != len(tt.want) {
				t.Errorf("got %d fields, want %d: got=%v", len(got), len(tt.want), got)
			}
			for k, wantV := range tt.want {
				if gotV, ok := got[k]; !ok || gotV != wantV {
					t.Errorf("key %q: got %q, want %q", k, got[k], wantV)
				}
			}
		})
	}
}

func TestResolveItemMapping_Homogeneous(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				NameField: "name",
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
		},
	}

	cfg := &importMappingConfig{
		Type: "person",
		Map:  map[string]string{"full_name": "name"},
	}

	item := map[string]interface{}{"full_name": "Freya", "email": "f@a.realm"}

	typeName, fieldMap, matchKey, err := resolveItemMapping(item, cfg, sch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typeName != "person" {
		t.Errorf("typeName: got %q, want %q", typeName, "person")
	}
	if fieldMap["full_name"] != "name" {
		t.Errorf("fieldMap: got %v", fieldMap)
	}
	if matchKey != "name" {
		t.Errorf("matchKey: got %q, want %q", matchKey, "name")
	}
}

func TestResolveItemMapping_Heterogeneous(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				NameField: "name",
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
			"project": {
				NameField: "name",
				Fields: map[string]*schema.FieldDefinition{
					"name": {Type: schema.FieldTypeString},
				},
			},
		},
	}

	cfg := &importMappingConfig{
		TypeField: "kind",
		Types: map[string]importTypeMapping{
			"contact": {
				Type: "person",
				Map:  map[string]string{"full_name": "name"},
			},
			"task": {
				Type: "project",
				Map:  map[string]string{"title": "name"},
			},
		},
	}

	// Test contact item
	contactItem := map[string]interface{}{"kind": "contact", "full_name": "Freya"}
	typeName, fieldMap, matchKey, err := resolveItemMapping(contactItem, cfg, sch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typeName != "person" {
		t.Errorf("typeName: got %q, want %q", typeName, "person")
	}
	if fieldMap["full_name"] != "name" {
		t.Errorf("fieldMap: got %v", fieldMap)
	}
	if matchKey != "name" {
		t.Errorf("matchKey: got %q, want %q", matchKey, "name")
	}

	// Test task item
	taskItem := map[string]interface{}{"kind": "task", "title": "Bifrost"}
	typeName, fieldMap, matchKey, err = resolveItemMapping(taskItem, cfg, sch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if typeName != "project" {
		t.Errorf("typeName: got %q, want %q", typeName, "project")
	}
	if fieldMap["title"] != "name" {
		t.Errorf("fieldMap: got %v", fieldMap)
	}

	// Test unknown source type
	unknownItem := map[string]interface{}{"kind": "unknown", "title": "X"}
	_, _, _, err = resolveItemMapping(unknownItem, cfg, sch)
	if err == nil {
		t.Error("expected error for unknown source type")
	}

	// Test missing type field
	missingItem := map[string]interface{}{"title": "X"}
	_, _, _, err = resolveItemMapping(missingItem, cfg, sch)
	if err == nil {
		t.Error("expected error for missing type field")
	}
}

func TestResolveItemMapping_ExplicitKey(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				NameField: "name",
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
		},
	}

	cfg := &importMappingConfig{
		Type: "person",
		Key:  "email",
		Map:  map[string]string{},
	}

	item := map[string]interface{}{"name": "Freya", "email": "f@a.realm"}

	_, _, matchKey, err := resolveItemMapping(item, cfg, sch)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if matchKey != "email" {
		t.Errorf("matchKey: got %q, want %q", matchKey, "email")
	}
}

func TestBuildMappingConfig_CLIArgs(t *testing.T) {
	// Reset global flags for test
	origMapping := importMapping
	origMapFlags := importMapFlags
	origKey := importKey
	defer func() {
		importMapping = origMapping
		importMapFlags = origMapFlags
		importKey = origKey
	}()

	importMapping = ""
	importMapFlags = []string{"full_name=name", "mail=email"}
	importKey = "email"

	cfg, err := buildMappingConfig([]string{"person"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != "person" {
		t.Errorf("Type: got %q, want %q", cfg.Type, "person")
	}
	if cfg.Key != "email" {
		t.Errorf("Key: got %q, want %q", cfg.Key, "email")
	}
	if cfg.Map["full_name"] != "name" {
		t.Errorf("Map[full_name]: got %q, want %q", cfg.Map["full_name"], "name")
	}
	if cfg.Map["mail"] != "email" {
		t.Errorf("Map[mail]: got %q, want %q", cfg.Map["mail"], "email")
	}
}

func TestBuildMappingConfig_NoType(t *testing.T) {
	origMapping := importMapping
	origMapFlags := importMapFlags
	origKey := importKey
	defer func() {
		importMapping = origMapping
		importMapFlags = origMapFlags
		importKey = origKey
	}()

	importMapping = ""
	importMapFlags = nil
	importKey = ""

	_, err := buildMappingConfig([]string{})
	if err == nil {
		t.Error("expected error when no type specified")
	}
}

func TestBuildMappingConfig_MappingFile(t *testing.T) {
	origMapping := importMapping
	origMapFlags := importMapFlags
	origKey := importKey
	defer func() {
		importMapping = origMapping
		importMapFlags = origMapFlags
		importKey = origKey
	}()

	// Write a temp mapping file
	dir := t.TempDir()
	mappingFile := filepath.Join(dir, "mapping.yaml")
	content := `type: person
key: name
map:
  full_name: name
  mail: email
`
	if err := os.WriteFile(mappingFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mapping file: %v", err)
	}

	importMapping = mappingFile
	importMapFlags = nil
	importKey = ""

	cfg, err := buildMappingConfig([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != "person" {
		t.Errorf("Type: got %q, want %q", cfg.Type, "person")
	}
	if cfg.Key != "name" {
		t.Errorf("Key: got %q, want %q", cfg.Key, "name")
	}
	if cfg.Map["full_name"] != "name" {
		t.Errorf("Map[full_name]: got %q, want %q", cfg.Map["full_name"], "name")
	}
}

func TestBuildMappingConfig_HeterogeneousMappingFile(t *testing.T) {
	origMapping := importMapping
	origMapFlags := importMapFlags
	origKey := importKey
	defer func() {
		importMapping = origMapping
		importMapFlags = origMapFlags
		importKey = origKey
	}()

	dir := t.TempDir()
	mappingFile := filepath.Join(dir, "mapping.yaml")
	content := `type_field: kind
types:
  contact:
    type: person
    key: name
    map:
      full_name: name
  task:
    type: project
    map:
      title: name
`
	if err := os.WriteFile(mappingFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mapping file: %v", err)
	}

	importMapping = mappingFile
	importMapFlags = nil
	importKey = ""

	cfg, err := buildMappingConfig([]string{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.TypeField != "kind" {
		t.Errorf("TypeField: got %q, want %q", cfg.TypeField, "kind")
	}
	if len(cfg.Types) != 2 {
		t.Errorf("Types: got %d entries, want 2", len(cfg.Types))
	}

	contactMapping, ok := cfg.Types["contact"]
	if !ok {
		t.Fatal("missing contact mapping")
	}
	if contactMapping.Type != "person" {
		t.Errorf("contact.Type: got %q, want %q", contactMapping.Type, "person")
	}
	if contactMapping.Key != "name" {
		t.Errorf("contact.Key: got %q, want %q", contactMapping.Key, "name")
	}
}

func TestValidateMappingTypes(t *testing.T) {
	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person":  {},
			"project": {},
		},
	}

	// Valid homogeneous
	err := validateMappingTypes(&importMappingConfig{Type: "person"}, sch)
	if err != nil {
		t.Errorf("unexpected error for valid type: %v", err)
	}

	// Invalid homogeneous
	err = validateMappingTypes(&importMappingConfig{Type: "unknown"}, sch)
	if err == nil {
		t.Error("expected error for unknown type")
	}

	// Valid heterogeneous
	err = validateMappingTypes(&importMappingConfig{
		TypeField: "kind",
		Types: map[string]importTypeMapping{
			"contact": {Type: "person"},
			"task":    {Type: "project"},
		},
	}, sch)
	if err != nil {
		t.Errorf("unexpected error for valid types: %v", err)
	}

	// Invalid heterogeneous
	err = validateMappingTypes(&importMappingConfig{
		TypeField: "kind",
		Types: map[string]importTypeMapping{
			"contact": {Type: "person"},
			"task":    {Type: "nonexistent"},
		},
	}, sch)
	if err == nil {
		t.Error("expected error for unknown mapped type")
	}

	// Built-in type is valid
	err = validateMappingTypes(&importMappingConfig{Type: "page"}, sch)
	if err != nil {
		t.Errorf("unexpected error for built-in type: %v", err)
	}
}

func TestBuildMappingConfig_CLIOverridesMappingFile(t *testing.T) {
	origMapping := importMapping
	origMapFlags := importMapFlags
	origKey := importKey
	defer func() {
		importMapping = origMapping
		importMapFlags = origMapFlags
		importKey = origKey
	}()

	// Write a mapping file
	dir := t.TempDir()
	mappingFile := filepath.Join(dir, "mapping.yaml")
	content := `type: person
key: name
map:
  full_name: name
`
	if err := os.WriteFile(mappingFile, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write mapping file: %v", err)
	}

	importMapping = mappingFile
	importMapFlags = []string{"mail=email"}
	importKey = "email"

	// CLI type arg overrides file type
	cfg, err := buildMappingConfig([]string{"project"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.Type != "project" {
		t.Errorf("Type: got %q, want %q (CLI should override file)", cfg.Type, "project")
	}
	if cfg.Key != "email" {
		t.Errorf("Key: got %q, want %q (CLI should override file)", cfg.Key, "email")
	}
	// CLI --map flags should be merged
	if cfg.Map["full_name"] != "name" {
		t.Errorf("Map[full_name] from file: got %q, want %q", cfg.Map["full_name"], "name")
	}
	if cfg.Map["mail"] != "email" {
		t.Errorf("Map[mail] from CLI: got %q, want %q", cfg.Map["mail"], "email")
	}
}
