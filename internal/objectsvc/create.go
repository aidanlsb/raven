package objectsvc

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
)

type CreateRequest struct {
	VaultPath        string
	TypeName         string
	Title            string
	TargetPath       string
	FieldValues      map[string]string
	TypedFieldValues map[string]schema.FieldValue
	VaultConfig      *config.VaultConfig
	Schema           *schema.Schema
	ObjectsRoot      string
	PagesRoot        string
	TemplateDir      string
	TemplateID       string
}

type CreateResult struct {
	FilePath     string
	RelativePath string
}

func Create(req CreateRequest) (*CreateResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.TypeName) == "" {
		return nil, newError(ErrorInvalidInput, "type is required", "", nil, nil)
	}
	if strings.TrimSpace(req.Title) == "" {
		return nil, newError(ErrorInvalidInput, "title is required", "Usage: rvn new <type> <title> --json", nil, nil)
	}
	if strings.ContainsAny(req.Title, `/\`) {
		return nil, newError(ErrorInvalidInput, "title cannot contain path separators", "Provide a plain title without path separators", nil, nil)
	}

	typeDef, typeExists := req.Schema.Types[req.TypeName]
	if !typeExists && !schema.IsBuiltinType(req.TypeName) {
		var typeNames []string
		for name := range req.Schema.Types {
			typeNames = append(typeNames, name)
		}
		sort.Strings(typeNames)
		return nil, newError(
			ErrorTypeNotFound,
			fmt.Sprintf("type '%s' not found", req.TypeName),
			fmt.Sprintf("Available types: %s", strings.Join(typeNames, ", ")),
			map[string]interface{}{"available_types": typeNames},
			nil,
		)
	}

	targetPath := req.TargetPath
	if strings.TrimSpace(targetPath) == "" {
		targetPath = req.Title
	}

	fieldValues := make(map[string]string, len(req.FieldValues)+len(req.TypedFieldValues))
	for key, value := range req.FieldValues {
		fieldValues[key] = value
	}

	if typeDef != nil && typeDef.NameField != "" {
		if _, provided := fieldValues[typeDef.NameField]; !provided {
			if _, typedProvided := req.TypedFieldValues[typeDef.NameField]; !typedProvided && req.Title != "" {
				fieldValues[typeDef.NameField] = req.Title
			}
		}
	}

	for key, value := range req.TypedFieldValues {
		fieldValues[key] = fieldmutation.SerializeFieldValueLiteral(value)
	}

	var missingFields []string
	var fieldDetails []map[string]interface{}
	if typeDef != nil {
		var fieldNames []string
		for name := range typeDef.Fields {
			fieldNames = append(fieldNames, name)
		}
		sort.Strings(fieldNames)

		for _, fieldName := range fieldNames {
			fieldDef := typeDef.Fields[fieldName]
			if fieldDef == nil || !fieldDef.Required {
				continue
			}
			if _, ok := fieldValues[fieldName]; ok {
				continue
			}
			if fieldDef.Default != nil {
				fieldValues[fieldName] = fmt.Sprintf("%v", fieldDef.Default)
				continue
			}
			missingFields = append(missingFields, fieldName)
			detail := map[string]interface{}{
				"name":     fieldName,
				"type":     string(fieldDef.Type),
				"required": true,
			}
			if len(fieldDef.Values) > 0 {
				detail["values"] = fieldDef.Values
			}
			fieldDetails = append(fieldDetails, detail)
		}
	}

	if len(missingFields) > 0 {
		details := map[string]interface{}{
			"missing_fields": fieldDetails,
			"type":           req.TypeName,
			"title":          req.Title,
			"retry_with": map[string]interface{}{
				"type":  req.TypeName,
				"title": req.Title,
				"field": buildFieldTemplate(missingFields),
			},
		}
		if typeDef != nil && typeDef.NameField != "" {
			details["name_field"] = typeDef.NameField
			details["name_field_hint"] = fmt.Sprintf("The title argument auto-populates the '%s' field", typeDef.NameField)
		}
		return nil, newError(
			ErrorRequiredField,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missingFields, ", ")),
			fmt.Sprintf("Retry the same call with: field: {%s}", buildFieldTemplateExample(missingFields)),
			details,
			nil,
		)
	}

	resolvedSlugPath := pages.SlugifyPath(
		pages.ResolveTargetPathWithRoots(targetPath, req.TypeName, req.Schema, req.ObjectsRoot, req.PagesRoot),
	)
	if pages.Exists(req.VaultPath, pages.ResolveTargetPathWithRoots(targetPath, req.TypeName, req.Schema, req.ObjectsRoot, req.PagesRoot)) {
		return nil, newError(
			ErrorFileExists,
			fmt.Sprintf("file already exists: %s.md", resolvedSlugPath),
			"Choose a different title, or use `rvn open <reference>` to open the existing object",
			nil,
			nil,
		)
	}

	templateOverride, err := schema.ResolveTypeTemplateFile(req.Schema, req.TypeName, req.TemplateID)
	if err != nil {
		return nil, newError(ErrorInvalidInput, err.Error(), "Use `rvn schema template list --type <type_name>` to see available template IDs", nil, err)
	}

	validatedFields, resolvedFields, _, err := fieldmutation.PrepareValidatedFieldMutation(
		req.TypeName,
		nil,
		fieldValues,
		req.Schema,
		nil,
		&fieldmutation.RefValidationContext{
			VaultPath:   req.VaultPath,
			VaultConfig: req.VaultConfig,
		},
	)
	if err != nil {
		return nil, newError(ErrorValidationFailed, err.Error(), "Ensure values match the schema field types for this object", nil, err)
	}

	fieldValues = resolvedFields
	for key, value := range validatedFields {
		fieldValues[key] = fieldmutation.SerializeFieldValueLiteral(value)
	}

	result, err := pages.Create(pages.CreateOptions{
		VaultPath:        req.VaultPath,
		TypeName:         req.TypeName,
		Title:            req.Title,
		TargetPath:       targetPath,
		Fields:           fieldValues,
		Schema:           req.Schema,
		TemplateOverride: templateOverride,
		TemplateDir:      req.TemplateDir,
		ObjectsRoot:      req.ObjectsRoot,
		PagesRoot:        req.PagesRoot,
	})
	if err != nil {
		return nil, newError(ErrorFileWrite, "failed to create object", "", nil, err)
	}

	return &CreateResult{
		FilePath:     result.FilePath,
		RelativePath: result.RelativePath,
	}, nil
}

func buildFieldTemplateExample(missingFields []string) string {
	parts := make([]string, 0, len(missingFields))
	for _, f := range missingFields {
		parts = append(parts, fmt.Sprintf(`"%s": "<value>"`, f))
	}
	return strings.Join(parts, ", ")
}
