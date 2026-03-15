package cli

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/schema"
)

type fieldValidationError = fieldmutation.ValidationError
type unknownFieldMutationError = fieldmutation.UnknownFieldMutationError

func warningMessagesToWarnings(messages []string) []Warning {
	warnings := make([]Warning, 0, len(messages))
	for _, message := range messages {
		warnings = append(warnings, Warning{
			Code:    WarnUnknownField,
			Message: message,
		})
	}
	return warnings
}

func parseFieldValuesJSON(raw string) (map[string]schema.FieldValue, error) {
	return fieldmutation.ParseFieldValuesJSON(raw)
}
