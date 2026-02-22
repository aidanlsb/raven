package schema

import (
	"fmt"
	"sort"
	"strings"
)

// ResolveTypeTemplateFile resolves a template file path for a type.
//
// Selection order:
// 1) If templateID is provided, resolve that template ID from type.templates.
// 2) Else, if default_template is set, resolve that template ID.
// 3) Else, fallback to legacy type.template (if present).
// 4) Else, no template ("").
func ResolveTypeTemplateFile(sch *Schema, typeName, templateID string) (string, error) {
	if sch == nil {
		return "", nil
	}

	typeDef, ok := sch.Types[typeName]
	if !ok || typeDef == nil {
		return "", nil
	}

	selectedID := strings.TrimSpace(templateID)
	if selectedID != "" {
		return resolveTypeTemplateID(sch, typeName, typeDef, selectedID)
	}

	if strings.TrimSpace(typeDef.DefaultTemplate) != "" {
		return resolveTypeTemplateID(sch, typeName, typeDef, strings.TrimSpace(typeDef.DefaultTemplate))
	}

	// Backward-compatible fallback.
	if strings.TrimSpace(typeDef.Template) != "" {
		return strings.TrimSpace(typeDef.Template), nil
	}

	return "", nil
}

func resolveTypeTemplateID(sch *Schema, typeName string, typeDef *TypeDefinition, templateID string) (string, error) {
	if len(typeDef.Templates) == 0 {
		return "", fmt.Errorf("type %q has no templates configured", typeName)
	}

	if !containsTemplateID(typeDef.Templates, templateID) {
		return "", fmt.Errorf("type %q does not include template %q (available: %s)", typeName, templateID, formatTemplateIDs(typeDef.Templates))
	}

	templateDef, ok := sch.Templates[templateID]
	if !ok || templateDef == nil {
		return "", fmt.Errorf("type %q references unknown template %q", typeName, templateID)
	}

	file := strings.TrimSpace(templateDef.File)
	if file == "" {
		return "", fmt.Errorf("template %q has no file path configured", templateID)
	}

	return file, nil
}

func formatTemplateIDs(templateIDs []string) string {
	if len(templateIDs) == 0 {
		return "none"
	}
	normalized := make([]string, 0, len(templateIDs))
	for _, id := range templateIDs {
		normalized = append(normalized, strings.TrimSpace(id))
	}
	sort.Strings(normalized)
	return strings.Join(normalized, ", ")
}

func containsTemplateID(templateIDs []string, candidate string) bool {
	for _, id := range templateIDs {
		if strings.TrimSpace(id) == candidate {
			return true
		}
	}
	return false
}
