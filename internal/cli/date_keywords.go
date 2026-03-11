package cli

import (
	"time"

	"github.com/aidanlsb/raven/internal/dates"
	"github.com/aidanlsb/raven/internal/schema"
)

func resolveRelativeDateKeyword(value string) (string, bool) {
	resolved, ok := dates.ResolveRelativeDateKeyword(value, time.Now(), time.Monday)
	if !ok || resolved.Kind != dates.RelativeDateInstant {
		return "", false
	}
	return resolved.Date.Format(dates.DateLayout), true
}

func resolveDateKeywordForTraitValue(value string, traitDef *schema.TraitDefinition) string {
	if traitDef == nil || traitDef.Type != schema.FieldTypeDate {
		return value
	}

	if resolved, ok := resolveRelativeDateKeyword(value); ok {
		return resolved
	}

	return value
}
