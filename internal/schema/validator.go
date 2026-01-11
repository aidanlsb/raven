package schema

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/dates"
)

// ValidationError represents a field validation error.
type ValidationError struct {
	Field   string
	Message string
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("Field '%s': %s", e.Field, e.Message)
}

// ValidateFields validates a set of fields against a type's field definitions.
func ValidateFields(fields map[string]FieldValue, fieldDefs map[string]*FieldDefinition, schema *Schema) []ValidationError {
	var errors []ValidationError

	// Check required fields are present
	for name, def := range fieldDefs {
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
		// Skip reserved fields
		if name == "id" || name == "type" || name == "alias" {
			continue
		}

		if def, ok := fieldDefs[name]; ok {
			if err := validateFieldValue(name, value, def); err != nil {
				errors = append(errors, ValidationError{
					Field:   name,
					Message: err.Error(),
				})
			}
		}
		// Note: Unknown fields are allowed (schema is not strict)
	}

	return errors
}

func validateFieldValue(name string, value FieldValue, def *FieldDefinition) error {
	if value.IsNull() {
		return nil // Null is always valid (required check handles missing fields)
	}

	switch def.Type {
	case FieldTypeString:
		if _, ok := value.AsString(); !ok {
			return fmt.Errorf("expected string")
		}

	case FieldTypeStringArray:
		arr, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("expected array of strings")
		}
		for _, v := range arr {
			if _, ok := v.AsString(); !ok {
				return fmt.Errorf("expected array of strings")
			}
		}

	case FieldTypeNumber:
		n, ok := value.AsNumber()
		if !ok {
			return fmt.Errorf("expected number")
		}
		if def.Min != nil && n < *def.Min {
			return fmt.Errorf("value %v is below minimum %v", n, *def.Min)
		}
		if def.Max != nil && n > *def.Max {
			return fmt.Errorf("value %v is above maximum %v", n, *def.Max)
		}

	case FieldTypeNumberArray:
		arr, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("expected array of numbers")
		}
		for _, v := range arr {
			if _, ok := v.AsNumber(); !ok {
				return fmt.Errorf("expected array of numbers")
			}
		}

	case FieldTypeDate:
		s, ok := value.AsString()
		if !ok {
			return fmt.Errorf("expected date")
		}
		if !isValidDate(s) {
			return fmt.Errorf("invalid date format, expected YYYY-MM-DD")
		}

	case FieldTypeDateArray:
		arr, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("expected array of dates")
		}
		for _, v := range arr {
			s, ok := v.AsString()
			if !ok || !isValidDate(s) {
				return fmt.Errorf("expected array of dates")
			}
		}

	case FieldTypeDatetime:
		s, ok := value.AsString()
		if !ok {
			return fmt.Errorf("expected datetime")
		}
		if !isValidDatetime(s) {
			return fmt.Errorf("invalid datetime format")
		}

	case FieldTypeEnum:
		s, ok := value.AsString()
		if !ok {
			return fmt.Errorf("expected enum value (string)")
		}
		if def.Values == nil {
			return fmt.Errorf("enum type missing 'values' definition")
		}
		found := false
		for _, allowed := range def.Values {
			if s == allowed {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid enum value '%s', expected one of: %v", s, def.Values)
		}

	case FieldTypeBool:
		if _, ok := value.AsBool(); !ok {
			return fmt.Errorf("expected boolean")
		}

	case FieldTypeRef:
		// Allow both ref and string as ref
		if _, ok := value.AsRef(); !ok {
			if _, ok := value.AsString(); !ok {
				return fmt.Errorf("expected reference")
			}
		}

	case FieldTypeRefArray:
		arr, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("expected array of references")
		}
		for _, v := range arr {
			if !v.IsRef() {
				if _, ok := v.AsString(); !ok {
					return fmt.Errorf("expected array of references")
				}
			}
		}
	}

	return nil
}

func isValidDate(s string) bool {
	return dates.IsValidDate(s)
}

func isValidDatetime(s string) bool {
	return dates.IsValidDatetime(s)
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

	if fieldDef.Type != FieldTypeString {
		return fmt.Errorf("name_field '%s' must be a string field, got '%s'", typeDef.NameField, fieldDef.Type)
	}

	return nil
}

// ValidateSchema performs comprehensive validation of a schema.
// Returns a list of issues found.
func ValidateSchema(sch *Schema) []string {
	var issues []string

	for typeName, typeDef := range sch.Types {
		// Skip built-in types
		if IsBuiltinType(typeName) {
			continue
		}

		// Validate name_field
		if err := ValidateNameField(typeDef); err != nil {
			issues = append(issues, fmt.Sprintf("Type '%s': %s", typeName, err.Error()))
		}

		// Validate ref field targets
		if typeDef.Fields != nil {
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef != nil && (fieldDef.Type == FieldTypeRef || fieldDef.Type == FieldTypeRefArray) && fieldDef.Target != "" {
					if _, exists := sch.Types[fieldDef.Target]; !exists {
						issues = append(issues, fmt.Sprintf("Type '%s' field '%s' references unknown type '%s'", typeName, fieldName, fieldDef.Target))
					}
				}
			}
		}
	}

	return issues
}
