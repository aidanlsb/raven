package schema

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/aidanlsb/raven/internal/dates"
)

// ValidateTraitValue validates a single trait value against the trait definition.
// It assumes a value is present (non-bare trait usage).
func ValidateTraitValue(def *TraitDefinition, value FieldValue) error {
	if def == nil {
		return nil
	}

	switch normalizedTraitType(def.Type, def.IsBoolean()) {
	case FieldTypeBool:
		if _, ok := value.AsBool(); ok {
			return nil
		}
		if s, ok := value.AsString(); ok && (s == "true" || s == "false") {
			return nil
		}
		return fmt.Errorf("invalid boolean value %q (expected true or false)", traitValueDisplay(value))
	case FieldTypeString:
		if _, ok := value.AsString(); ok {
			return nil
		}
		return fmt.Errorf("expected string value")
	case FieldTypeNumber:
		if _, ok := value.AsNumber(); ok {
			return nil
		}
		if s, ok := value.AsString(); ok {
			if _, err := strconv.ParseFloat(strings.TrimSpace(s), 64); err == nil {
				return nil
			}
		}
		return fmt.Errorf("invalid number value %q", traitValueDisplay(value))
	case FieldTypeURL:
		s, ok := value.AsString()
		if !ok {
			return fmt.Errorf("expected URL value")
		}
		if err := validateURLString(s); err != nil {
			return err
		}
		return nil
	case FieldTypeDate:
		s, ok := value.AsString()
		if !ok || !dates.IsValidDate(s) {
			return fmt.Errorf("invalid date format %q (expected YYYY-MM-DD)", traitValueDisplay(value))
		}
		return nil
	case FieldTypeDatetime:
		s, ok := value.AsString()
		if !ok || !dates.IsValidDatetime(s) {
			return fmt.Errorf("invalid datetime format %q (expected YYYY-MM-DDTHH:MM or YYYY-MM-DDTHH:MM:SS)", traitValueDisplay(value))
		}
		return nil
	case FieldTypeEnum:
		s, ok := value.AsString()
		if !ok {
			return fmt.Errorf("expected enum value")
		}
		if len(def.Values) == 0 {
			return fmt.Errorf("enum trait has no allowed values defined")
		}
		for _, allowed := range def.Values {
			if s == allowed {
				return nil
			}
		}
		return fmt.Errorf("invalid enum value %q (allowed: %v)", s, def.Values)
	case FieldTypeRef:
		if target, ok := refTargetFromFieldValue(value); ok && target != "" {
			return nil
		}
		return fmt.Errorf("expected reference value")
	default:
		return fmt.Errorf("unknown trait type %q", def.Type)
	}
}

func normalizedTraitType(fieldType FieldType, isBoolean bool) FieldType {
	if isBoolean {
		return FieldTypeBool
	}
	return normalizeFieldType(fieldType)
}

func traitValueDisplay(value FieldValue) string {
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
