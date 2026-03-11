package cli

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type fieldValidationError = fieldmutation.ValidationError
type unknownFieldMutationError = fieldmutation.UnknownFieldMutationError

func prepareValidatedFieldMutation(
	objectType string,
	existingFields map[string]schema.FieldValue,
	updates map[string]string,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
) (map[string]schema.FieldValue, map[string]string, []string, error) {
	return fieldmutation.PrepareValidatedFieldMutation(objectType, existingFields, updates, sch, allowedUnknown)
}

func prepareValidatedFrontmatterMutation(
	content string,
	fm *parser.Frontmatter,
	objectType string,
	updates map[string]string,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
) (string, map[string]string, []string, error) {
	return fieldmutation.PrepareValidatedFrontmatterMutation(content, fm, objectType, updates, sch, allowedUnknown)
}

func prepareValidatedFieldMutationValues(
	objectType string,
	existingFields map[string]schema.FieldValue,
	updates map[string]schema.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
) (map[string]schema.FieldValue, []string, error) {
	return fieldmutation.PrepareValidatedFieldMutationValues(objectType, existingFields, updates, sch, allowedUnknown)
}

func prepareValidatedFrontmatterMutationValues(
	content string,
	fm *parser.Frontmatter,
	objectType string,
	updates map[string]schema.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
) (string, []string, error) {
	return fieldmutation.PrepareValidatedFrontmatterMutationValues(content, fm, objectType, updates, sch, allowedUnknown)
}

func detectUnknownFieldMutationByNames(
	objectType string,
	sch *schema.Schema,
	fieldNames []string,
	allowedUnknown map[string]bool,
) *unknownFieldMutationError {
	return fieldmutation.DetectUnknownFieldMutationByNames(objectType, sch, fieldNames, allowedUnknown)
}

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

// buildFieldTemplate creates a template object showing required field names.
// This is included in error responses so agents can see exactly what structure to provide.
func buildFieldTemplate(missingFields []string) map[string]string {
	result := make(map[string]string)
	for _, f := range missingFields {
		result[f] = "<value>"
	}
	return result
}
