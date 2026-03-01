package cli

import (
	"strings"
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

func resolveDateKeywordList(value string) (string, bool) {
	trimmed := strings.TrimSpace(value)
	if !strings.HasPrefix(trimmed, "[") || !strings.HasSuffix(trimmed, "]") {
		return "", false
	}

	inner := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
	if inner == "" {
		return "", false
	}

	parts := strings.Split(inner, ",")
	changed := false
	for i, part := range parts {
		part = strings.TrimSpace(part)
		unquoted := strings.Trim(part, `"'`)
		if resolved, ok := resolveRelativeDateKeyword(unquoted); ok {
			parts[i] = resolved
			changed = true
		} else {
			parts[i] = part
		}
	}

	if !changed {
		return "", false
	}

	return "[" + strings.Join(parts, ", ") + "]", true
}

func resolveDateKeywordForFieldValue(value string, fieldDef *schema.FieldDefinition) string {
	if fieldDef == nil {
		return value
	}

	switch fieldDef.Type {
	case schema.FieldTypeDate:
		if resolved, ok := resolveRelativeDateKeyword(value); ok {
			return resolved
		}
	case schema.FieldTypeDateArray:
		if resolved, ok := resolveDateKeywordList(value); ok {
			return resolved
		}
	}

	return value
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
