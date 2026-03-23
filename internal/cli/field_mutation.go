package cli

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/schema"
)

func parseFieldValuesJSON(raw string) (map[string]schema.FieldValue, error) {
	return fieldmutation.ParseFieldValuesJSON(raw)
}
