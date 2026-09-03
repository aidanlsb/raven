package schema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldvalue"
)

// ValidateTraitValue validates a single trait value against the trait definition.
// It assumes a value is present (non-bare trait usage).
func ValidateTraitValue(def *TraitDefinition, value fieldvalue.FieldValue) error {
	if def == nil {
		return nil
	}

	traitType := normalizedTraitType(def.Type, def.IsBoolean())

	// Handle array types by extracting scalar type and validating elements
	scalarType := traitType
	isArray := false
	if elementType, ok := arrayTypeToScalar[traitType]; ok {
		scalarType = elementType
		isArray = true
	}

	// Get validator from table
	validator, ok := validatorTable[scalarType]
	if !ok {
		return fmt.Errorf("unknown trait type %q", def.Type)
	}

	// Build a temporary FieldDefinition for extraCheck calls
	tempDef := &FieldDefinition{
		Type:   scalarType,
		Values: def.Values,
	}

	if isArray {
		items, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("expected array value")
		}
		for _, item := range items {
			// Special handling for bool: allow string "true"/"false"
			if scalarType == FieldTypeBool {
				if err := validateTraitBoolValue(item); err != nil {
					return err
				}
				continue
			}
			// Special handling for number: allow parseable strings
			if scalarType == FieldTypeNumber {
				if err := validateTraitNumberValue(item); err != nil {
					return err
				}
				continue
			}
			// Standard validation for other types
			if !validator.scalarChecker(item) {
				return traitValidationError(scalarType, item, validator.scalarError, false)
			}
			if validator.extraCheck != nil {
				msg := validator.extraCheck(item, tempDef)
				if msg != "" {
					return fmt.Errorf("%s", msg)
				}
			}
		}
		return nil
	}

	// Scalar validation with trait-specific handling
	if scalarType == FieldTypeBool {
		return validateTraitBoolValue(value)
	}
	if scalarType == FieldTypeNumber {
		return validateTraitNumberValue(value)
	}
	if !validator.scalarChecker(value) {
		return traitValidationError(scalarType, value, validator.scalarError, true)
	}
	if validator.extraCheck != nil {
		msg := validator.extraCheck(value, tempDef)
		if msg != "" {
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}

// validateTraitBoolValue validates a boolean trait value, allowing string "true"/"false".
func validateTraitBoolValue(value fieldvalue.FieldValue) error {
	if _, ok := value.AsBool(); ok {
		return nil
	}
	if s, ok := value.AsString(); ok && (s == "true" || s == "false") {
		return nil
	}
	return fmt.Errorf("invalid boolean value %q (expected true or false)", traitValueDisplay(value))
}

// validateTraitNumberValue validates a number trait value, allowing parseable strings.
func validateTraitNumberValue(value fieldvalue.FieldValue) error {
	if _, ok := value.AsNumber(); ok {
		return nil
	}
	if s, ok := value.AsString(); ok {
		if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
			return nil
		}
	}
	return fmt.Errorf("invalid number value %q", traitValueDisplay(value))
}

// traitValidationError returns trait-specific error messages with better formatting.
func traitValidationError(fieldType FieldType, value fieldvalue.FieldValue, defaultMsg string, isScalar bool) error {
	switch fieldType {
	case FieldTypeDate:
		return fmt.Errorf("invalid date format %q (expected YYYY-MM-DD)", traitValueDisplay(value))
	case FieldTypeDatetime:
		return fmt.Errorf("invalid datetime format %q (expected YYYY-MM-DDTHH:MM or YYYY-MM-DDTHH:MM:SS)", traitValueDisplay(value))
	case FieldTypeEnum:
		if isScalar {
			return fmt.Errorf("expected enum value")
		}
		return fmt.Errorf("%s", defaultMsg)
	case FieldTypeRef:
		if isScalar {
			return fmt.Errorf("expected reference value")
		}
		return fmt.Errorf("%s", defaultMsg)
	default:
		return fmt.Errorf("%s", defaultMsg)
	}
}

func normalizedTraitType(fieldType FieldType, isBoolean bool) FieldType {
	if isBoolean {
		return FieldTypeBool
	}
	return normalizeFieldType(fieldType)
}

func traitValueDisplay(value fieldvalue.FieldValue) string {
	if s, ok := value.AsString(); ok {
		return s
	}
	if b, ok := value.AsBool(); ok {
		if b {
			return "true"
		}
		return "false"
	}
	if n, ok := value.AsNumber(); ok {
		return strconv.FormatFloat(n, 'f', -1, 64)
	}
	return fmt.Sprintf("%v", value.Raw())
}
