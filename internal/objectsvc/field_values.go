package objectsvc

import (
	"sort"

	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
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

func requiredFieldGapsValues(typeDef *schema.TypeDefinition, fields map[string]schema.FieldValue) []string {
	if typeDef == nil {
		return nil
	}

	var missing []string
	for fieldName, fieldDef := range typeDef.Fields {
		if fieldDef == nil || !fieldDef.Required {
			continue
		}
		if _, ok := fields[fieldName]; ok {
			continue
		}
		if fieldDef.Default != nil {
			fields[fieldName] = parser.FieldValueFromYAML(fieldDef.Default)
			continue
		}
		missing = append(missing, fieldName)
	}
	sort.Strings(missing)
	return missing
}

func fieldValueMatchesValue(existing, input schema.FieldValue) bool {
	return fieldmutation.SerializeFieldValueLiteral(existing) == fieldmutation.SerializeFieldValueLiteral(input)
}
