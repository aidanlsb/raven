package schema

import (
	"testing"

	"github.com/aidanlsb/raven/internal/fieldvalue"
)

func TestValidateTraitValue(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		def     *TraitDefinition
		value   fieldvalue.FieldValue
		wantErr bool
	}{
		{
			name:    "boolean true string",
			def:     &TraitDefinition{Type: FieldTypeBool},
			value:   fieldvalue.String("true"),
			wantErr: false,
		},
		{
			name:    "boolean invalid string",
			def:     &TraitDefinition{Type: FieldTypeBool},
			value:   fieldvalue.String("yes"),
			wantErr: true,
		},
		{
			name:    "number valid numeric string",
			def:     &TraitDefinition{Type: FieldTypeNumber},
			value:   fieldvalue.String("3.5"),
			wantErr: false,
		},
		{
			name:    "number invalid",
			def:     &TraitDefinition{Type: FieldTypeNumber},
			value:   fieldvalue.String("abc"),
			wantErr: true,
		},
		{
			name:    "url valid",
			def:     &TraitDefinition{Type: FieldTypeURL},
			value:   fieldvalue.String("https://example.com"),
			wantErr: false,
		},
		{
			name:    "url invalid",
			def:     &TraitDefinition{Type: FieldTypeURL},
			value:   fieldvalue.String("example.com"),
			wantErr: true,
		},
		{
			name:    "date valid",
			def:     &TraitDefinition{Type: FieldTypeDate},
			value:   fieldvalue.String("2026-02-21"),
			wantErr: false,
		},
		{
			name:    "date invalid",
			def:     &TraitDefinition{Type: FieldTypeDate},
			value:   fieldvalue.String("2026-99-99"),
			wantErr: true,
		},
		{
			name:    "datetime valid",
			def:     &TraitDefinition{Type: FieldTypeDatetime},
			value:   fieldvalue.String("2026-02-21T09:30"),
			wantErr: false,
		},
		{
			name:    "datetime invalid",
			def:     &TraitDefinition{Type: FieldTypeDatetime},
			value:   fieldvalue.String("tomorrow"),
			wantErr: true,
		},
		{
			name:    "enum valid",
			def:     &TraitDefinition{Type: FieldTypeEnum, Values: []string{"low", "medium", "high"}},
			value:   fieldvalue.String("high"),
			wantErr: false,
		},
		{
			name:    "enum invalid",
			def:     &TraitDefinition{Type: FieldTypeEnum, Values: []string{"low", "medium", "high"}},
			value:   fieldvalue.String("critical"),
			wantErr: true,
		},
		{
			name:    "ref valid wikilink parsed",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   fieldvalue.Ref("people/freya"),
			wantErr: false,
		},
		{
			name:    "ref valid bare string",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   fieldvalue.String("people/freya"),
			wantErr: false,
		},
		{
			name:    "ref invalid empty",
			def:     &TraitDefinition{Type: FieldTypeRef},
			value:   fieldvalue.String(""),
			wantErr: true,
		},
		{
			name:    "legacy boolean alias",
			def:     &TraitDefinition{Type: "boolean"},
			value:   fieldvalue.String("false"),
			wantErr: false,
		},
		{
			name:    "string array valid",
			def:     &TraitDefinition{Type: FieldTypeStringArray},
			value:   fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("raven"), fieldvalue.String("skills")}),
			wantErr: false,
		},
		{
			name:    "number array valid numeric strings",
			def:     &TraitDefinition{Type: FieldTypeNumberArray},
			value:   fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("1"), fieldvalue.String("2.5")}),
			wantErr: false,
		},
		{
			name:    "bool array valid string booleans",
			def:     &TraitDefinition{Type: FieldTypeBoolArray},
			value:   fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("true"), fieldvalue.String("false")}),
			wantErr: false,
		},
		{
			name:    "enum array invalid member",
			def:     &TraitDefinition{Type: FieldTypeEnumArray, Values: []string{"low", "medium", "high"}},
			value:   fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.String("high"), fieldvalue.String("critical")}),
			wantErr: true,
		},
		{
			name:    "ref array valid",
			def:     &TraitDefinition{Type: FieldTypeRefArray},
			value:   fieldvalue.Array([]fieldvalue.FieldValue{fieldvalue.Ref("people/freya"), fieldvalue.String("people/loki")}),
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
