package schema

import "testing"

func TestNew(t *testing.T) {
	t.Parallel()
	s := New()

	if s.Types == nil {
		t.Fatal("Types should not be nil")
	}
	if s.Traits == nil {
		t.Fatal("Traits should not be nil")
	}
	if s.Core == nil {
		t.Fatal("Core should not be nil")
	}

	for _, typeName := range []string{"page", "section", "date"} {
		if _, ok := s.Types[typeName]; !ok {
			t.Errorf("expected %q type to exist", typeName)
		}
	}

	section := s.Types["section"]
	if _, ok := section.Fields["title"]; !ok {
		t.Error("section should have 'title' field")
	}
	if _, ok := section.Fields["level"]; !ok {
		t.Error("section should have 'level' field")
	}
}

func TestTraitDefinitionIsBoolean(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		traitDef TraitDefinition
		want     bool
	}{
		{"empty type", TraitDefinition{Type: ""}, true},
		{"bool type", TraitDefinition{Type: FieldTypeBool}, true},
		{"boolean type", TraitDefinition{Type: "boolean"}, true},
		{"date type", TraitDefinition{Type: FieldTypeDate}, false},
		{"string type", TraitDefinition{Type: FieldTypeString}, false},
		{"enum type", TraitDefinition{Type: FieldTypeEnum}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.traitDef.IsBoolean(); got != tt.want {
				t.Errorf("IsBoolean() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestIsBuiltinType(t *testing.T) {
	t.Parallel()
	tests := []struct {
		typeName string
		want     bool
	}{
		{"page", true},
		{"section", true},
		{"date", true},
		{"person", false},
		{"project", false},
		{"meeting", false},
		{"", false},
	}

	for _, tt := range tests {
		if got := IsBuiltinType(tt.typeName); got != tt.want {
			t.Errorf("IsBuiltinType(%q) = %v, want %v", tt.typeName, got, tt.want)
		}
	}
}
