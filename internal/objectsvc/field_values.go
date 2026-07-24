package objectsvc

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/schema"
)

func cloneFieldValues(values map[string]fieldvalue.FieldValue) map[string]fieldvalue.FieldValue {
	if len(values) == 0 {
		return map[string]fieldvalue.FieldValue{}
	}

	out := make(map[string]fieldvalue.FieldValue, len(values))
	for key, value := range values {
		out[key] = value
	}
	return out
}

func ensureNameFieldValue(fields map[string]fieldvalue.FieldValue, typeDef *schema.TypeDefinition, title string) {
	if typeDef == nil || typeDef.NameField == "" || title == "" {
		return
	}
	if _, exists := fields[typeDef.NameField]; exists {
		return
	}
	fields[typeDef.NameField] = fieldvalue.String(title)
}

func fieldValueMatchesValue(existing, input fieldvalue.FieldValue) bool {
	return fieldmutation.SerializeFieldValueLiteral(existing) == fieldmutation.SerializeFieldValueLiteral(input)
}
