package importsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/schema"
)

func TestApplyFieldMappings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		item     map[string]interface{}
		fieldMap map[string]string
		want     map[string]interface{}
	}{
		{
			name:     "no mappings pass through",
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
			name:     "nil field map passes through",
			item:     map[string]interface{}{"name": "Thor"},
			fieldMap: nil,
			want:     map[string]interface{}{"name": "Thor"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := ApplyFieldMappings(tt.item, tt.fieldMap)
			assertMapEqual(t, got, tt.want)
		})
	}
}

func TestMatchKeyValue(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		mapped   map[string]interface{}
		matchKey string
		wantVal  string
		wantOK   bool
	}{
		{name: "string value", mapped: map[string]interface{}{"name": "Freya"}, matchKey: "name", wantVal: "Freya", wantOK: true},
		{name: "missing key", mapped: map[string]interface{}{"email": "f@a.realm"}, matchKey: "name", wantOK: false},
		{name: "empty string", mapped: map[string]interface{}{"name": ""}, matchKey: "name", wantOK: false},
		{name: "numeric value", mapped: map[string]interface{}{"id": float64(42)}, matchKey: "id", wantVal: "42", wantOK: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			gotVal, gotOK := MatchKeyValue(tt.mapped, tt.matchKey)
			if gotOK != tt.wantOK {
				t.Fatalf("ok = %v, want %v", gotOK, tt.wantOK)
			}
			if gotVal != tt.wantVal {
				t.Fatalf("value = %q, want %q", gotVal, tt.wantVal)
			}
		})
	}
}

func TestFieldsToStringMap(t *testing.T) {
	t.Parallel()

	fields := map[string]interface{}{
		"type":   "person",
		"name":   "Freya",
		"rating": float64(5),
		"score":  float64(3.14),
		"active": true,
		"tags":   []interface{}{"go", "rust"},
		"notes":  nil,
	}
	want := map[string]string{
		"name":   "Freya",
		"rating": "5",
		"score":  "3.14",
		"active": "true",
		"tags":   "[go, rust]",
	}

	got := FieldsToStringMap(fields, "person")
	assertStringMapEqual(t, got, want)
}

func TestResolveItemMapping(t *testing.T) {
	t.Parallel()

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {
				NameField: "name",
				Fields: map[string]*schema.FieldDefinition{
					"name":  {Type: schema.FieldTypeString},
					"email": {Type: schema.FieldTypeString},
				},
			},
			"project": {
				NameField: "title",
				Fields: map[string]*schema.FieldDefinition{
					"title": {Type: schema.FieldTypeString},
				},
			},
		},
	}

	t.Run("homogeneous mapping uses schema name field", func(t *testing.T) {
		cfg := &MappingConfig{
			Type: "person",
			Map:  map[string]string{"full_name": "name"},
		}
		itemCfg, err := ResolveItemMapping(map[string]interface{}{"full_name": "Freya"}, cfg, sch)
		if err != nil {
			t.Fatalf("ResolveItemMapping returned error: %v", err)
		}
		if itemCfg.TypeName != "person" || itemCfg.FieldMap["full_name"] != "name" || itemCfg.MatchKey != "name" {
			t.Fatalf("item config = %#v", itemCfg)
		}
	})

	t.Run("heterogeneous mapping selects source type", func(t *testing.T) {
		cfg := &MappingConfig{
			TypeField: "kind",
			Types: map[string]TypeMapping{
				"contact": {Type: "person", Key: "email", Map: map[string]string{"full_name": "name"}},
				"work":    {Type: "project", Map: map[string]string{"label": "title"}},
			},
		}
		itemCfg, err := ResolveItemMapping(map[string]interface{}{"kind": "work", "label": "Raven"}, cfg, sch)
		if err != nil {
			t.Fatalf("ResolveItemMapping returned error: %v", err)
		}
		if itemCfg.TypeName != "project" || itemCfg.MatchKey != "title" || itemCfg.FieldMap["label"] != "title" {
			t.Fatalf("item config = %#v", itemCfg)
		}
	})

	t.Run("missing match key is rejected", func(t *testing.T) {
		cfg := &MappingConfig{Type: "unknown", Map: map[string]string{}}
		_, err := ResolveItemMapping(map[string]interface{}{"name": "Freya"}, cfg, sch)
		if err == nil {
			t.Fatal("expected missing match key error")
		}
	})
}

func TestBuildMappingConfig(t *testing.T) {
	t.Parallel()

	mappingPath := filepath.Join(t.TempDir(), "mapping.yaml")
	if err := os.WriteFile(mappingPath, []byte(`type: person
key: external_id
content_field: body
map:
  full_name: name
`), 0o644); err != nil {
		t.Fatalf("write mapping file: %v", err)
	}

	cfg, err := BuildMappingConfig(BuildMappingConfigRequest{
		MappingFilePath: mappingPath,
		CLIType:         "project",
		MapFlags:        []string{"owner=lead"},
		Key:             "slug",
		ContentField:    "notes",
	})
	if err != nil {
		t.Fatalf("BuildMappingConfig returned error: %v", err)
	}
	if cfg.Type != "project" || cfg.Key != "slug" || cfg.ContentField != "notes" {
		t.Fatalf("config scalar overrides not applied: %#v", cfg)
	}
	wantMap := map[string]string{"full_name": "name", "owner": "lead"}
	assertStringMapEqual(t, cfg.Map, wantMap)

	_, err = BuildMappingConfig(BuildMappingConfigRequest{MapFlags: []string{"bad"}})
	if err == nil {
		t.Fatal("expected invalid --map format error")
	}
}

func TestValidateMappingTypes(t *testing.T) {
	t.Parallel()

	sch := &schema.Schema{
		Types: map[string]*schema.TypeDefinition{
			"person": {Fields: map[string]*schema.FieldDefinition{}},
		},
	}
	if err := ValidateMappingTypes(&MappingConfig{Type: "person"}, sch); err != nil {
		t.Fatalf("ValidateMappingTypes known type returned error: %v", err)
	}
	if err := ValidateMappingTypes(&MappingConfig{Type: "page"}, sch); err != nil {
		t.Fatalf("ValidateMappingTypes built-in type returned error: %v", err)
	}
	if err := ValidateMappingTypes(&MappingConfig{Type: "unknown"}, sch); err == nil {
		t.Fatal("expected unknown type error")
	}
	if err := ValidateMappingTypes(&MappingConfig{
		TypeField: "kind",
		Types: map[string]TypeMapping{
			"contact": {Type: "missing"},
		},
	}, sch); err == nil {
		t.Fatal("expected unknown heterogeneous type error")
	}
}

func TestExtractContentFieldAndReplaceBodyContent(t *testing.T) {
	t.Parallel()

	mapped := map[string]interface{}{"name": "Freya", "bio": "Queen"}
	if got := ExtractContentField(mapped, "bio"); got != "Queen" {
		t.Fatalf("ExtractContentField = %q, want Queen", got)
	}
	if _, ok := mapped["bio"]; ok {
		t.Fatal("expected content field to be removed from mapped fields")
	}

	file := "---\ntype: person\nname: Freya\n---\n\n# Old\n"
	got := ReplaceBodyContent(file, "# New\n")
	want := "---\ntype: person\nname: Freya\n---\n\n# New\n"
	if got != want {
		t.Fatalf("ReplaceBodyContent() = %q, want %q", got, want)
	}
}

func assertMapEqual(t *testing.T, got, want map[string]interface{}) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d fields, want %d: got=%v", len(got), len(want), got)
	}
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || gotValue != wantValue {
			t.Fatalf("key %q: got %v, want %v", key, got[key], wantValue)
		}
	}
}

func assertStringMapEqual(t *testing.T, got, want map[string]string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d fields, want %d: got=%v", len(got), len(want), got)
	}
	for key, wantValue := range want {
		if gotValue, ok := got[key]; !ok || gotValue != wantValue {
			t.Fatalf("key %q: got %q, want %q", key, got[key], wantValue)
		}
	}
}
