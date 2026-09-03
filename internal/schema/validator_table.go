package schema

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/fieldvalue"
)

// fieldValidator defines validation logic for a single FieldType.
type fieldValidator struct {
	// scalarChecker validates a single scalar value. Returns true if valid, false if type mismatch.
	scalarChecker func(fieldvalue.FieldValue) bool
	// scalarError returns the error message when scalarChecker returns false.
	scalarError string
	// arrayError returns the error message when array element fails scalarChecker.
	arrayError string
	// extraCheck performs additional validation (min/max, enum values, etc.) after type check passes.
	// Returns error message or empty string if valid.
	extraCheck func(fieldvalue.FieldValue, *FieldDefinition) string
}

// validatorTable maps each FieldType to its validation logic.
var validatorTable = map[FieldType]fieldValidator{
	FieldTypeString: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsString()
			return ok
		},
		scalarError: "expected string",
		arrayError:  "expected array of strings",
	},
	FieldTypeNumber: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsNumber()
			return ok
		},
		scalarError: "expected number",
		arrayError:  "expected array of numbers",
		extraCheck: func(v fieldvalue.FieldValue, def *FieldDefinition) string {
			n, _ := v.AsNumber()
			if def.Min != nil && n < *def.Min {
				return fmt.Sprintf("value %v is below minimum %v", n, *def.Min)
			}
			if def.Max != nil && n > *def.Max {
				return fmt.Sprintf("value %v is above maximum %v", n, *def.Max)
			}
			return ""
		},
	},
	FieldTypeURL: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsString()
			return ok
		},
		scalarError: "expected URL",
		arrayError:  "expected array of URLs",
		extraCheck: func(v fieldvalue.FieldValue, def *FieldDefinition) string {
			s, _ := v.AsString()
			if err := validateURLString(s); err != nil {
				return err.Error()
			}
			return ""
		},
	},
	FieldTypeDate: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsString()
			return ok
		},
		scalarError: "expected date",
		arrayError:  "expected array of dates",
		extraCheck: func(v fieldvalue.FieldValue, def *FieldDefinition) string {
			s, _ := v.AsString()
			if !dates.IsValidDate(s) {
				return "invalid date format, expected YYYY-MM-DD"
			}
			return ""
		},
	},
	FieldTypeDatetime: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsString()
			return ok
		},
		scalarError: "expected datetime",
		arrayError:  "expected array of datetimes",
		extraCheck: func(v fieldvalue.FieldValue, def *FieldDefinition) string {
			s, _ := v.AsString()
			if !dates.IsValidDatetime(s) {
				return "invalid datetime format"
			}
			return ""
		},
	},
	FieldTypeEnum: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsString()
			return ok
		},
		scalarError: "expected enum value (string)",
		arrayError:  "expected array of enum values",
		extraCheck: func(v fieldvalue.FieldValue, def *FieldDefinition) string {
			if def.Values == nil {
				return "enum type missing 'values' definition"
			}
			s, _ := v.AsString()
			for _, allowed := range def.Values {
				if s == allowed {
					return ""
				}
			}
			return fmt.Sprintf("invalid enum value '%s', expected one of: %v", s, def.Values)
		},
	},
	FieldTypeBool: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := v.AsBool()
			return ok
		},
		scalarError: "expected boolean",
		arrayError:  "expected array of booleans",
	},
	FieldTypeRef: {
		scalarChecker: func(v fieldvalue.FieldValue) bool {
			_, ok := refTargetFromFieldValue(v)
			return ok
		},
		scalarError: "expected reference",
		arrayError:  "expected array of references",
	},
}

// arrayTypeToScalar maps array FieldTypes to their scalar element types.
var arrayTypeToScalar = map[FieldType]FieldType{
	FieldTypeStringArray:   FieldTypeString,
	FieldTypeNumberArray:   FieldTypeNumber,
	FieldTypeURLArray:      FieldTypeURL,
	FieldTypeDateArray:     FieldTypeDate,
	FieldTypeDatetimeArray: FieldTypeDatetime,
	FieldTypeEnumArray:     FieldTypeEnum,
	FieldTypeBoolArray:     FieldTypeBool,
	FieldTypeRefArray:      FieldTypeRef,
}

// validateFieldValue validates a field value against its definition using the validator table.
func validateFieldValue(name string, value fieldvalue.FieldValue, def *FieldDefinition) error {
	if value.IsNull() {
		return nil
	}

	// Handle array types by extracting scalar type and validating elements
	scalarType := def.Type
	isArray := false
	if elementType, ok := arrayTypeToScalar[def.Type]; ok {
		scalarType = elementType
		isArray = true
	}

	validator, ok := validatorTable[scalarType]
	if !ok {
		return fmt.Errorf("unsupported field type '%s'", def.Type)
	}

	if isArray {
		arr, ok := value.AsArray()
		if !ok {
			return fmt.Errorf("%s", validator.arrayError)
		}
		for _, elem := range arr {
			if !validator.scalarChecker(elem) {
				return fmt.Errorf("%s", validator.arrayError)
			}
			if validator.extraCheck != nil {
				msg := validator.extraCheck(elem, def)
				if msg != "" {
					return fmt.Errorf("%s", msg)
				}
			}
		}
		return nil
	}

	// Scalar validation
	if !validator.scalarChecker(value) {
		return fmt.Errorf("%s", validator.scalarError)
	}
	if validator.extraCheck != nil {
		msg := validator.extraCheck(value, def)
		if msg != "" {
			return fmt.Errorf("%s", msg)
		}
	}
	return nil
}
