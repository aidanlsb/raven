package objectsvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type requiredFieldGap struct {
	Name   string
	Type   schema.FieldType
	Values []string
}

type createPageRequest struct {
	VaultPath        string
	TypeName         string
	Title            string
	TargetPath       string
	Fields           map[string]schema.FieldValue
	Schema           *schema.Schema
	TemplateOverride string
	TemplateDir      string
	VaultConfig      *config.VaultConfig
	ObjectsRoot      string
	PagesRoot        string
}

func lookupTypeDefinitionForCreate(sch *schema.Schema, typeName string) (*schema.TypeDefinition, error) {
	typeDef, typeExists := sch.Types[typeName]
	if typeExists || schema.IsBuiltinType(typeName) {
		return typeDef, nil
	}

	typeNames := make([]string, 0, len(sch.Types))
	for name := range sch.Types {
		typeNames = append(typeNames, name)
	}
	sort.Strings(typeNames)

	return nil, newError(
		ErrorTypeNotFound,
		fmt.Sprintf("type '%s' not found", typeName),
		fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")),
		map[string]interface{}{"available_types": typeNames},
		nil,
	)
}

func normalizedCreateFieldValues(values map[string]schema.FieldValue, typeDef *schema.TypeDefinition, title string) map[string]schema.FieldValue {
	fieldValues := cloneFieldValues(values)
	ensureNameFieldValue(fieldValues, typeDef, title)
	return fieldValues
}

func requiredFieldGaps(typeDef *schema.TypeDefinition, fields map[string]schema.FieldValue) []requiredFieldGap {
	if typeDef == nil {
		return nil
	}

	fieldNames := make([]string, 0, len(typeDef.Fields))
	for fieldName := range typeDef.Fields {
		fieldNames = append(fieldNames, fieldName)
	}
	sort.Strings(fieldNames)

	missing := make([]requiredFieldGap, 0)
	for _, fieldName := range fieldNames {
		fieldDef := typeDef.Fields[fieldName]
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

		gap := requiredFieldGap{
			Name: fieldName,
			Type: fieldDef.Type,
		}
		if len(fieldDef.Values) > 0 {
			gap.Values = append([]string(nil), fieldDef.Values...)
		}
		missing = append(missing, gap)
	}

	return missing
}

func requiredFieldGapNames(gaps []requiredFieldGap) []string {
	names := make([]string, 0, len(gaps))
	for _, gap := range gaps {
		names = append(names, gap.Name)
	}
	return names
}

func requiredFieldGapDetails(gaps []requiredFieldGap) []map[string]interface{} {
	details := make([]map[string]interface{}, 0, len(gaps))
	for _, gap := range gaps {
		detail := map[string]interface{}{
			"name":     gap.Name,
			"type":     string(gap.Type),
			"required": true,
		}
		if len(gap.Values) > 0 {
			detail["values"] = gap.Values
		}
		details = append(details, detail)
	}
	return details
}

func createRefValidationContext(vaultPath string, vaultCfg *config.VaultConfig) *fieldmutation.RefValidationContext {
	return &fieldmutation.RefValidationContext{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
	}
}

func validateCreateFieldValues(
	typeName string,
	fields map[string]schema.FieldValue,
	sch *schema.Schema,
	allowedUnknown map[string]bool,
	refCtx *fieldmutation.RefValidationContext,
) (map[string]schema.FieldValue, []string, error) {
	return fieldmutation.PrepareValidatedFieldMutationValues(typeName, nil, fields, sch, allowedUnknown, refCtx)
}

func createObjectPage(req createPageRequest) (*pages.CreateResult, error) {
	result, err := pages.Create(pages.CreateOptions{
		VaultPath:         req.VaultPath,
		TypeName:          req.TypeName,
		Title:             req.Title,
		TargetPath:        req.TargetPath,
		Fields:            req.Fields,
		Schema:            req.Schema,
		TemplateOverride:  req.TemplateOverride,
		TemplateDir:       req.TemplateDir,
		ProtectedPrefixes: protectedPrefixes(req.VaultConfig),
		ObjectsRoot:       req.ObjectsRoot,
		PagesRoot:         req.PagesRoot,
	})
	if err != nil {
		return nil, newError(ErrorFileWrite, "failed to create object", "", nil, err)
	}
	return result, nil
}

func protectedPrefixes(vaultCfg *config.VaultConfig) []string {
	if vaultCfg == nil {
		return nil
	}
	return vaultCfg.ProtectedPrefixes
}
