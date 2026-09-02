package schema

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
)

// MsgUnknownFrontmatterKey is the stable ValidationError.Message for undeclared keys.
const MsgUnknownFrontmatterKey = "unknown frontmatter key"

// ValidationError represents a field validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("Field '%s': %s", e.Field, e.Message)
}

// IsReservedFrontmatterKey reports whether name is a built-in frontmatter key
// that is never treated as a schema field.
//
// Note: `id` is intentionally NOT reserved. Raven object identity is
// path-derived; alternate names are expressed via `alias`. A frontmatter `id`
// key is therefore treated as an ordinary (unknown) field so that leftover
// `id:` values surface honestly as `unknown_frontmatter_key`.
func IsReservedFrontmatterKey(name string) bool {
	switch name {
	case "type", "alias":
		return true
	default:
		return false
	}
}

// UnknownFrontmatterKeys returns sorted field names that are neither reserved
// nor declared on the type. allowedExtra skips additional keys (e.g. mutation
// allowlists). This is the single strictness gate for object frontmatter keys.
func UnknownFrontmatterKeys(
	fields map[string]fieldvalue.FieldValue,
	fieldDefs map[string]*FieldDefinition,
	allowedExtra map[string]bool,
) []string {
	if len(fields) == 0 {
		return nil
	}

	unknown := make([]string, 0)
	for name := range fields {
		if IsReservedFrontmatterKey(name) {
			continue
		}
		if allowedExtra != nil && allowedExtra[name] {
			continue
		}
		if _, ok := fieldDefs[name]; ok {
			continue
		}
		unknown = append(unknown, name)
	}
	sort.Strings(unknown)
	return unknown
}

// IndexableFields returns the subset of fields that should be stored in the
// index for a known type: reserved keys plus schema-defined fields. When
// fieldDefs is nil (unknown type), all fields are returned unchanged.
func IndexableFields(fields map[string]fieldvalue.FieldValue, fieldDefs map[string]*FieldDefinition) map[string]fieldvalue.FieldValue {
	if fields == nil || fieldDefs == nil {
		return fields
	}

	out := make(map[string]fieldvalue.FieldValue, len(fields))
	for name, value := range fields {
		if IsReservedFrontmatterKey(name) {
			out[name] = value
			continue
		}
		if _, ok := fieldDefs[name]; ok {
			out[name] = value
		}
	}
	return out
}

// ValidateFields validates a set of fields against a type's field definitions.
// Unknown (non-reserved) frontmatter keys are validation errors.
func ValidateFields(fields map[string]fieldvalue.FieldValue, fieldDefs map[string]*FieldDefinition, schema *Schema) []ValidationError {
	var errors []ValidationError
	invalidDefs := make(map[string]struct{})

	// Check required fields are present
	for name, def := range fieldDefs {
		if def == nil {
			errors = append(errors, ValidationError{
				Field:   name,
				Message: "Field definition is null in schema",
			})
			invalidDefs[name] = struct{}{}
			continue
		}
		if def.Required {
			val, exists := fields[name]
			if !exists || val.IsNull() {
				if def.Default == nil {
					errors = append(errors, ValidationError{
						Field:   name,
						Message: "Required field is missing",
					})
				}
			}
		}
	}

	// Validate each provided field
	for name, value := range fields {
		if IsReservedFrontmatterKey(name) {
			continue
		}

		if def, ok := fieldDefs[name]; ok {
			if def == nil {
				if _, reported := invalidDefs[name]; !reported {
					errors = append(errors, ValidationError{
						Field:   name,
						Message: "Field definition is null in schema",
					})
				}
				continue
			}
			if err := validateFieldValue(name, value, def); err != nil {
				errors = append(errors, ValidationError{
					Field:   name,
					Message: err.Error(),
				})
			}
			continue
		}

		errors = append(errors, ValidationError{
			Field:   name,
			Message: MsgUnknownFrontmatterKey,
		})
	}

	return errors
}

// validateFieldValue is defined in validator_table.go

// refTargetFromFieldValue extracts a reference target string from various value formats.
func refTargetFromFieldValue(value fieldvalue.FieldValue) (string, bool) {
	if r, ok := value.AsRef(); ok && r != "" {
		return r, true
	}
	if s, ok := value.AsString(); ok && s != "" {
		return s, true
	}
	if arr, ok := value.AsArray(); ok && len(arr) == 1 {
		if s, ok := arr[0].AsString(); ok && s != "" {
			return s, true
		}
		if innerArr, ok := arr[0].AsArray(); ok && len(innerArr) == 1 {
			if s, ok := innerArr[0].AsString(); ok && s != "" {
				return s, true
			}
		}
	}
	return "", false
}

// ValidateNameField checks that a type's name_field is valid.
// Returns an error if the name_field references a non-existent field or a non-string field.
func ValidateNameField(typeDef *TypeDefinition) error {
	if typeDef.NameField == "" {
		return nil // No name_field configured is valid
	}

	fieldDef, exists := typeDef.Fields[typeDef.NameField]
	if !exists {
		return fmt.Errorf("name_field '%s' references non-existent field", typeDef.NameField)
	}
	if fieldDef == nil {
		return fmt.Errorf("name_field '%s' references null field definition", typeDef.NameField)
	}

	if fieldDef.Type != FieldTypeString {
		return fmt.Errorf("name_field '%s' must be a string field, got '%s'", typeDef.NameField, fieldDef.Type)
	}

	return nil
}

// ValidateSchema performs comprehensive validation of a schema.
// Returns a list of issues found.
func ValidateSchema(sch *Schema) []string {
	var issues []string
	validTypes := ValidFieldTypes()

	for templateID, templateDef := range sch.Templates {
		if templateDef == nil {
			issues = append(issues, fmt.Sprintf("Template '%s' is null; expected an object with a file field", templateID))
			continue
		}
		if templateDef.File == "" {
			issues = append(issues, fmt.Sprintf("Template '%s' must define a non-empty file path", templateID))
		}
	}

	for typeName, typeDef := range sch.Types {
		// Built-in type definitions are runtime-owned and validated via core config.
		if IsBuiltinType(typeName) {
			continue
		}
		if typeDef == nil {
			issues = append(issues, fmt.Sprintf("Type '%s' must be an object", typeName))
			continue
		}

		// Validate name_field
		if err := ValidateNameField(typeDef); err != nil {
			issues = append(issues, fmt.Sprintf("Type '%s': %s", typeName, err.Error()))
		}

		// Validate ref field targets
		if typeDef.Fields != nil {
			for fieldName, fieldDef := range typeDef.Fields {
				issues = append(issues, validateSchemaFieldDefinition(typeName, fieldName, fieldDef, sch, validTypes)...)
			}
		}

		seenTemplateIDs := make(map[string]struct{})
		for _, templateID := range typeDef.Templates {
			if templateID == "" {
				issues = append(issues, fmt.Sprintf("Type '%s' templates cannot contain empty template IDs", typeName))
				continue
			}
			if _, seen := seenTemplateIDs[templateID]; seen {
				issues = append(issues, fmt.Sprintf("Type '%s' templates contains duplicate template ID '%s'", typeName, templateID))
			}
			seenTemplateIDs[templateID] = struct{}{}
			if _, exists := sch.Templates[templateID]; !exists {
				issues = append(issues, fmt.Sprintf("Type '%s' references unknown template '%s'", typeName, templateID))
			}
		}
		if typeDef.DefaultTemplate != "" {
			if _, ok := seenTemplateIDs[typeDef.DefaultTemplate]; !ok {
				issues = append(issues, fmt.Sprintf("Type '%s' default_template '%s' is not included in type templates", typeName, typeDef.DefaultTemplate))
			}
		}
	}
	for traitName, traitDef := range sch.Traits {
		issues = append(issues, validateSchemaTraitDefinition(traitName, traitDef)...)
	}
	for coreName, coreDef := range sch.Core {
		if !IsBuiltinType(coreName) {
			issues = append(issues, fmt.Sprintf("Unknown core type '%s'", coreName))
			continue
		}
		if coreDef == nil {
			issues = append(issues, fmt.Sprintf("Core type '%s' must be an object", coreName))
			continue
		}

		if coreName == "section" {
			if len(coreDef.Templates) > 0 || strings.TrimSpace(coreDef.DefaultTemplate) != "" {
				issues = append(issues, "Core type 'section' does not support template configuration")
			}
			continue
		}

		seenTemplateIDs := make(map[string]struct{})
		for _, templateID := range coreDef.Templates {
			if templateID == "" {
				issues = append(issues, fmt.Sprintf("Core type '%s' templates cannot contain empty template IDs", coreName))
				continue
			}
			if _, seen := seenTemplateIDs[templateID]; seen {
				issues = append(issues, fmt.Sprintf("Core type '%s' templates contains duplicate template ID '%s'", coreName, templateID))
			}
			seenTemplateIDs[templateID] = struct{}{}
			if _, exists := sch.Templates[templateID]; !exists {
				issues = append(issues, fmt.Sprintf("Core type '%s' references unknown template '%s'", coreName, templateID))
			}
		}
		if coreDef.DefaultTemplate != "" {
			if _, ok := seenTemplateIDs[coreDef.DefaultTemplate]; !ok {
				issues = append(issues, fmt.Sprintf("Core type '%s' default_template '%s' is not included in core templates", coreName, coreDef.DefaultTemplate))
			}
		}
	}

	return issues
}

func validateSchemaFieldDefinition(typeName, fieldName string, fieldDef *FieldDefinition, sch *Schema, validTypes string) []string {
	if fieldDef == nil {
		return []string{fmt.Sprintf("Type '%s' field '%s' must be an object", typeName, fieldName)}
	}

	var issues []string
	if !IsValidFieldType(fieldDef.Type) {
		return append(issues, fmt.Sprintf("Type '%s' field '%s' has unknown field type '%s' (expected one of: %s)", typeName, fieldName, fieldDef.Type, validTypes))
	}
	if (fieldDef.Type == FieldTypeEnum || fieldDef.Type == FieldTypeEnumArray) && len(fieldDef.Values) == 0 {
		issues = append(issues, fmt.Sprintf("Type '%s' field '%s' of type '%s' must define at least one allowed value", typeName, fieldName, fieldDef.Type))
	}
	if (fieldDef.Type == FieldTypeRef || fieldDef.Type == FieldTypeRefArray) && fieldDef.Target != "" {
		if _, exists := sch.Types[fieldDef.Target]; !exists {
			issues = append(issues, fmt.Sprintf("Type '%s' field '%s' references unknown type '%s'", typeName, fieldName, fieldDef.Target))
		}
	}
	return issues
}

func validateSchemaTraitDefinition(traitName string, traitDef *TraitDefinition) []string {
	if traitDef == nil {
		return []string{fmt.Sprintf("Trait '%s' must be an object", traitName)}
	}

	var issues []string
	traitType := normalizedTraitType(traitDef.Type, traitDef.IsBoolean())
	if !IsValidTraitType(traitType) {
		return append(issues, fmt.Sprintf("Trait '%s' has unknown trait type '%s' (expected one of: %s)", traitName, traitDef.Type, ValidTraitTypes()))
	}
	if traitType == FieldTypeEnum && len(traitDef.Values) == 0 {
		issues = append(issues, fmt.Sprintf("Trait '%s' of type '%s' must define at least one allowed value", traitName, traitType))
	}
	return issues
}

func IsValidFieldType(fieldType FieldType) bool {
	switch fieldType {
	case FieldTypeString,
		FieldTypeStringArray,
		FieldTypeNumber,
		FieldTypeNumberArray,
		FieldTypeURL,
		FieldTypeURLArray,
		FieldTypeDate,
		FieldTypeDateArray,
		FieldTypeDatetime,
		FieldTypeDatetimeArray,
		FieldTypeEnum,
		FieldTypeEnumArray,
		FieldTypeBool,
		FieldTypeBoolArray,
		FieldTypeRef,
		FieldTypeRefArray:
		return true
	default:
		return false
	}
}

func IsValidTraitType(fieldType FieldType) bool {
	if fieldType == "boolean" {
		return true
	}
	return IsValidFieldType(fieldType)
}

func ValidFieldTypes() string {
	return "string, string[], number, number[], url, url[], date, date[], datetime, datetime[], enum, enum[], bool, bool[], ref, ref[]"
}

func ValidTraitTypes() string {
	return ValidFieldTypes() + ", boolean"
}
