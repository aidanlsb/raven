package commandimpl

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/schema"
)

func mergeFieldInputs(literalUpdates map[string]string, typedUpdates map[string]schema.FieldValue) map[string]schema.FieldValue {
	merged := fieldmutation.ParseFieldValueLiterals(literalUpdates)
	if len(typedUpdates) == 0 {
		return merged
	}
	if len(merged) == 0 {
		merged = make(map[string]schema.FieldValue, len(typedUpdates))
	}
	for key, value := range typedUpdates {
		merged[key] = value
	}
	return merged
}

func boolArgDefault(args map[string]any, key string, defaultValue bool) bool {
	if args == nil {
		return defaultValue
	}
	if _, ok := args[key]; !ok {
		return defaultValue
	}
	return boolArg(args, key)
}
