package cli

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func schemaFieldDefaultLiteral(fieldDef *schema.FieldDefinition) (string, bool) {
	if fieldDef == nil || fieldDef.Default == nil {
		return "", false
	}
	literal := parser.FormatFieldValueLiteral(parser.FieldValueFromYAML(fieldDef.Default))
	if literal == "" {
		return "", false
	}
	return literal, true
}

func schemaFieldPromptText(fieldName string, fieldDef *schema.FieldDefinition) string {
	requirement := "optional"
	if fieldDef != nil && fieldDef.Required {
		requirement = "required"
	}

	suffix := ", blank to skip"
	if defaultLiteral, ok := schemaFieldDefaultLiteral(fieldDef); ok {
		suffix = fmt.Sprintf(", enter for %s", defaultLiteral)
	} else if fieldDef != nil && fieldDef.Required {
		suffix = ""
	}

	return fmt.Sprintf("%s (%s%s): ", fieldName, requirement, suffix)
}

func resolveInteractiveSchemaFieldInput(fieldName string, fieldDef *schema.FieldDefinition, input string) (value string, set bool, err error) {
	input = strings.TrimSpace(input)
	if input != "" {
		return input, true, nil
	}

	if defaultLiteral, ok := schemaFieldDefaultLiteral(fieldDef); ok {
		return defaultLiteral, true, nil
	}

	if fieldDef != nil && fieldDef.Required {
		return "", false, handleErrorMsg(
			ErrRequiredFieldMissing,
			fmt.Sprintf("required field '%s' cannot be empty", fieldName),
			"Provide a non-empty value",
		)
	}

	return "", false, nil
}
