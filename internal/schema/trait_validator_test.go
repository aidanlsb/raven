package schema

import "testing"

func TestValidateTraitValue(t *testing.T) {
	tests := []struct {
		name    string
		def     *TraitDefinition
		value   FieldValue
		wantErr bool
	}{
		{
			name:    "boolean true string",
			def:     &TraitDefinition{Type: FieldTypeBool},
			value:   String("true"),
			wantErr: false,
		},
		{
			name:    "boolean invalid string",
			def:     &TraitDefinition{Type: FieldTypeBool},
			value:   String("yes"),
			wantErr: true,
		},
		{
			name:    "number valid numeric string",
			def:     &TraitDefinition{Type: FieldTypeNumber},
			value:   String("3.5"),
			wantErr: false,
		},
		{
			name:    "number invalid",
			def:     &TraitDefinition{Type: FieldTypeNumber},
			value:   String("abc"),
			wantErr: true,
		},
		{
			name:    "date valid",
			def:     &TraitDefinition{Type: FieldTypeDate},
			value:   String("2026-02-21"),
			wantErr: false,
		},
		{
			name:    "date invalid",
			def:     &TraitDefinition{Type: FieldTypeDate},
			value:   String("2026-99-99"),
			wantErr: true,
		},
		{
			name:    "datetime valid",
			def:     &TraitDefinition{Type: FieldTypeDatetime},
			value:   String("2026-02-21T09:30"),
			wantErr: false,
		},
		{
			name:    "datetime invalid",
			def:     &TraitDefinition{Type: FieldTypeDatetime},
			value:   String("tomorrow"),
			wantErr: true,
		},
		{
			name:    "enum valid",
			def:     &TraitDefinition{Type: FieldTypeEnum, Values: []string{"low", "medium", "high"}},
			value:   String("high"),
			wantErr: false,
		},
		{
			name:    "enum invalid",
			def:     &TraitDefinition{Type: FieldTypeEnum, Values: []string{"low", "medium", "high"}},
			value:   String("critical"),
			wantErr: true,
		},
		{
			name:    "ref valid wikilink parsed",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   Ref("people/freya"),
			wantErr: false,
		},
		{
			name:    "ref valid bare string",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   String("people/freya"),
			wantErr: false,
		},
		{
			name:    "ref invalid empty",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   String(""),
			wantErr: true,
		},
		{
			name:    "legacy boolean alias",
			def:     &TraitDefinition{Type: "boolean"},
			value:   String("false"),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateTraitValue(tt.def, tt.value)
			if tt.wantErr && err == nil {
				t.Fatalf("expected error, got nil")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
