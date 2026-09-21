package schemasvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
)

type FieldTypeValidation struct {
	Valid      bool
	BaseType   string
	IsArray    bool
	Error      string
	Suggestion string
	Examples   []string
	ValidTypes []string
	TargetHint string
}

func ValidateFieldTypeSpec(fieldType, target, values string, sch *schema.Schema) FieldTypeValidation {
	result := FieldTypeValidation{
		ValidTypes: scalarFieldTypeNames(),
	}

	base, isArray := parseCLIFieldType(fieldType)
	result.BaseType = string(base)
	result.IsArray = isArray

	if sch != nil {
		if _, isSchemaType := sch.Types[string(base)]; isSchemaType && !schema.IsScalarFieldType(base) {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", base)
			result.Suggestion = fmt.Sprintf("To reference objects of type '%s', use --type ref --target %s", base, base)
			if isArray {
				result.Examples = []string{
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", base, base),
				}
			} else {
				result.Examples = []string{
					fmt.Sprintf("--type ref --target %s  (single %s reference)", base, base),
					fmt.Sprintf("--type ref[] --target %s  (array of %s references)", base, base),
				}
			}
			return result
		}

		cleanType := strings.TrimSuffix(string(base), "[]")
		clean := schema.FieldType(cleanType)
		if _, isSchemaType := sch.Types[cleanType]; isSchemaType && !schema.IsScalarFieldType(clean) {
			result.Error = fmt.Sprintf("'%s' is a type name, not a field type", cleanType)
			result.Suggestion = fmt.Sprintf("To reference an array of '%s' objects, use --type ref[] --target %s", cleanType, cleanType)
			result.Examples = []string{
				fmt.Sprintf("--type ref[] --target %s", cleanType),
			}
			return result
		}
	}

	if !schema.IsScalarFieldType(base) {
		display := fieldType
		if display == "" {
			display = "string"
		}
		result.Error = fmt.Sprintf("'%s' is not a valid field type", display)
		result.Suggestion = "Valid types: string, number, url, date, datetime, bool, enum, ref (add [] suffix for arrays)"
		result.Examples = []string{
			"--type string        (text)",
			"--type string[]      (array of text, e.g., tags)",
			"--type url           (web link)",
			"--type ref --target person   (reference to a person)",
			"--type ref[] --target person (array of person references)",
			"--type enum --values a,b,c   (single choice from list)",
		}
		return result
	}

	if base.IsRef() && target == "" {
		result.Error = "ref fields require --target to specify which type they reference"
		result.Suggestion = "Add --target <type_name> to specify the referenced type"
		if sch != nil && len(sch.Types) > 0 {
			typeNames := schemaTypeNames(sch)
			if len(typeNames) > 3 {
				typeNames = typeNames[:3]
			}
			result.Examples = make([]string, 0, len(typeNames))
			for _, t := range typeNames {
				if isArray {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref[] --target %s", t))
				} else {
					result.Examples = append(result.Examples, fmt.Sprintf("--type ref --target %s", t))
				}
			}
		}
		result.TargetHint = "Available types can be listed with 'rvn schema types'"
		return result
	}

	if base.IsEnum() && values == "" {
		result.Error = "enum fields require --values to specify allowed values"
		result.Suggestion = "Add --values with comma-separated allowed values"
		result.Examples = []string{
			"--type enum --values active,paused,done",
			"--type enum[] --values red,green,blue  (allows multiple selections)",
		}
		return result
	}

	if target != "" && !base.IsRef() {
		display := fieldType
		if display == "" {
			display = "string"
		}
		result.Error = fmt.Sprintf("--target is only valid for ref fields, but type is '%s'", display)
		result.Suggestion = "Either change --type to ref (or ref[]) or remove --target"
		result.Examples = []string{
			fmt.Sprintf("--type ref --target %s  (single reference)", target),
			fmt.Sprintf("--type ref[] --target %s  (array of references)", target),
		}
		return result
	}

	if target != "" && sch != nil {
		if _, exists := sch.Types[target]; !exists {
			if !schema.IsBuiltinType(target) {
				result.Error = fmt.Sprintf("target type '%s' does not exist in schema", target)
				result.Suggestion = fmt.Sprintf("Either create the type first with 'rvn schema add type %s' or use an existing type", target)
				if len(sch.Types) > 0 {
					result.TargetHint = fmt.Sprintf("Existing types: %s", strings.Join(schemaTypeNames(sch), ", "))
				}
				return result
			}
		}
	}

	result.Valid = true
	return result
}

func parseCLIFieldType(raw string) (schema.FieldType, bool) {
	if raw == "" {
		raw = "string"
	}
	isArray := strings.HasSuffix(raw, "[]")
	base := schema.FieldType(normalizeFieldTypeAlias(strings.TrimSuffix(raw, "[]")))
	return base, isArray
}

func scalarFieldTypeNames() []string {
	scalars := schema.ScalarFieldTypes()
	names := make([]string, len(scalars))
	for i, t := range scalars {
		names[i] = string(t)
	}
	return names
}

func schemaTypeNames(sch *schema.Schema) []string {
	return SortedKeys(sch.Types)
}

func normalizeFieldTypeAlias(baseType string) string {
	switch strings.ToLower(baseType) {
	case "boolean":
		return "bool"
	default:
		return strings.ToLower(baseType)
	}
}
