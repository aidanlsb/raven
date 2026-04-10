package objectsvc

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/schema"
)

func cloneFieldValues(values map[string]schema.FieldValue) map[string]schema.FieldValue {
	if len(values) == 0 {
		return map[string]schema.FieldValue{}
	}

	out := make(map[string]schema.FieldValue, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func ensureNameFieldValue(fields map[string]schema.FieldValue, typeDef *schema.TypeDefinition, title string) {
	if typeDef == nil || typeDef.NameField == "" || title == "" {
		return
	}
	if _, exists := fields[typeDef.NameField]; exists {
		return
	}
	fields[typeDef.NameField] = schema.String(title)
}

func fieldValueMatchesValue(existing, input schema.FieldValue) bool {
	return fieldmutation.SerializeFieldValueLiteral(existing) == fieldmutation.SerializeFieldValueLiteral(input)
}
