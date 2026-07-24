package cli

import (
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
)

func parseFieldValuesJSON(raw string) (map[string]fieldvalue.FieldValue, error) {
	return fieldmutation.ParseFieldValuesJSON(raw)
}
