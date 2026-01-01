// Package schema handles schema loading and validation.
package schema

import "encoding/json"

// Schema represents the complete schema definition loaded from schema.yaml.
type Schema struct {
	Types  map[string]*TypeDefinition  `yaml:"types"`
	Traits map[string]*TraitDefinition `yaml:"traits"`
}

// NewSchema creates a new schema with built-in types.
func NewSchema() *Schema {
	return &Schema{
		Types: map[string]*TypeDefinition{
			// Built-in 'page' type as fallback for untyped files
			"page": {
				Fields: make(map[string]*FieldDefinition),
			},
			// Built-in 'section' type for headings without explicit types
			"section": {
				Fields: map[string]*FieldDefinition{
					"title": {
						Type: FieldTypeString,
					},
					"level": {
						Type: FieldTypeNumber,
						Min:  floatPtr(1),
						Max:  floatPtr(6),
					},
				},
			},
		},
		Traits: make(map[string]*TraitDefinition),
	}
}

// TypeDefinition defines a type (person, meeting, project, etc.).
type TypeDefinition struct {
	Fields map[string]*FieldDefinition `yaml:"fields"`
	Detect *DetectionRules             `yaml:"detect,omitempty"`
}

// TraitDefinition defines a trait (@task, @remind, @highlight, etc.).
type TraitDefinition struct {
	Fields map[string]*FieldDefinition `yaml:"fields"`
	CLI    *TraitCLIConfig             `yaml:"cli,omitempty"`
}

// TraitCLIConfig contains CLI configuration for a trait.
type TraitCLIConfig struct {
	// Alias is the command name (e.g., "tasks" creates `rvn tasks`).
	Alias string `yaml:"alias,omitempty"`

	// DefaultQuery is the default filter when using the alias.
	DefaultQuery string `yaml:"default_query,omitempty"`
}

// FieldDefinition defines a field within a type or trait.
type FieldDefinition struct {
	Type       FieldType   `yaml:"type"`
	Required   bool        `yaml:"required,omitempty"`
	Default    interface{} `yaml:"default,omitempty"`
	Values     []string    `yaml:"values,omitempty"`   // For enum types
	Target     string      `yaml:"target,omitempty"`   // For ref types
	Min        *float64    `yaml:"min,omitempty"`      // For number types
	Max        *float64    `yaml:"max,omitempty"`      // For number types
	Derived    string      `yaml:"derived,omitempty"`  // How to compute value
	Positional bool        `yaml:"positional,omitempty"` // For traits: positional argument
}

// FieldType represents the type of a field.
type FieldType string

const (
	FieldTypeString      FieldType = "string"
	FieldTypeStringArray FieldType = "string[]"
	FieldTypeNumber      FieldType = "number"
	FieldTypeNumberArray FieldType = "number[]"
	FieldTypeDate        FieldType = "date"
	FieldTypeDateArray   FieldType = "date[]"
	FieldTypeDatetime    FieldType = "datetime"
	FieldTypeEnum        FieldType = "enum"
	FieldTypeBool        FieldType = "bool"
	FieldTypeRef         FieldType = "ref"
	FieldTypeRefArray    FieldType = "ref[]"
)

// DetectionRules for auto-detecting type from file.
type DetectionRules struct {
	// PathPattern is a regex pattern to match against file path.
	PathPattern string `yaml:"path_pattern,omitempty"`

	// Attribute matches specific frontmatter fields/values.
	Attribute map[string]interface{} `yaml:"attribute,omitempty"`
}

// FieldValue represents a parsed field value.
type FieldValue struct {
	value interface{}
}

// NewFieldValue creates a new FieldValue.
func NewFieldValue(v interface{}) FieldValue {
	return FieldValue{value: v}
}

// String creates a string FieldValue.
func String(s string) FieldValue {
	return FieldValue{value: s}
}

// Number creates a number FieldValue.
func Number(n float64) FieldValue {
	return FieldValue{value: n}
}

// Bool creates a boolean FieldValue.
func Bool(b bool) FieldValue {
	return FieldValue{value: b}
}

// Date creates a date FieldValue.
func Date(s string) FieldValue {
	return FieldValue{value: dateValue{s}}
}

// Datetime creates a datetime FieldValue.
func Datetime(s string) FieldValue {
	return FieldValue{value: datetimeValue{s}}
}

// Ref creates a reference FieldValue.
func Ref(s string) FieldValue {
	return FieldValue{value: refValue{s}}
}

// Array creates an array FieldValue.
func Array(items []FieldValue) FieldValue {
	return FieldValue{value: items}
}

// Null creates a null FieldValue.
func Null() FieldValue {
	return FieldValue{value: nil}
}

// Internal types to distinguish different string-based values.
type dateValue struct{ s string }
type datetimeValue struct{ s string }
type refValue struct{ s string }

// IsNull returns true if the value is null.
func (fv FieldValue) IsNull() bool {
	return fv.value == nil
}

// AsString returns the value as a string, if possible.
func (fv FieldValue) AsString() (string, bool) {
	switch v := fv.value.(type) {
	case string:
		return v, true
	case dateValue:
		return v.s, true
	case datetimeValue:
		return v.s, true
	case refValue:
		return v.s, true
	}
	return "", false
}

// AsNumber returns the value as a number, if possible.
func (fv FieldValue) AsNumber() (float64, bool) {
	if n, ok := fv.value.(float64); ok {
		return n, true
	}
	return 0, false
}

// AsBool returns the value as a boolean, if possible.
func (fv FieldValue) AsBool() (bool, bool) {
	if b, ok := fv.value.(bool); ok {
		return b, true
	}
	return false, false
}

// AsArray returns the value as an array, if possible.
func (fv FieldValue) AsArray() ([]FieldValue, bool) {
	if arr, ok := fv.value.([]FieldValue); ok {
		return arr, true
	}
	return nil, false
}

// AsRef returns the value as a reference path, if possible.
func (fv FieldValue) AsRef() (string, bool) {
	if r, ok := fv.value.(refValue); ok {
		return r.s, true
	}
	return "", false
}

// IsRef returns true if this is a reference value.
func (fv FieldValue) IsRef() bool {
	_, ok := fv.value.(refValue)
	return ok
}

// Raw returns the underlying raw value.
func (fv FieldValue) Raw() interface{} {
	switch v := fv.value.(type) {
	case dateValue:
		return v.s
	case datetimeValue:
		return v.s
	case refValue:
		return v.s
	case []FieldValue:
		result := make([]interface{}, len(v))
		for i, item := range v {
			result[i] = item.Raw()
		}
		return result
	default:
		return v
	}
}

// MarshalJSON implements json.Marshaler.
func (fv FieldValue) MarshalJSON() ([]byte, error) {
	return json.Marshal(fv.Raw())
}

// Helper function to create a float64 pointer.
func floatPtr(f float64) *float64 {
	return &f
}
