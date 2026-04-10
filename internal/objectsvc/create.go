package objectsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
)

type CreateRequest struct {
	VaultPath   string
	TypeName    string
	Title       string
	TargetPath  string
	FieldValues map[string]schema.FieldValue
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	ObjectsRoot string
	PagesRoot   string
	TemplateDir string
	TemplateID  string
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

	typeDef, err := lookupTypeDefinitionForCreate(req.Schema, req.TypeName)
	if err != nil {
		return nil, err
	}

	targetPath := req.TargetPath
	if strings.TrimSpace(targetPath) == "" {
		targetPath = req.Title
	}

	fieldValues := normalizedCreateFieldValues(req.FieldValues, typeDef, req.Title)
	missingFields := requiredFieldGaps(typeDef, fieldValues)

	if len(missingFields) > 0 {
		missingNames := requiredFieldGapNames(missingFields)
		details := map[string]interface{}{
			"missing_fields": requiredFieldGapDetails(missingFields),
			"type":           req.TypeName,
			"title":          req.Title,
			"retry_with": map[string]interface{}{
				"type":  req.TypeName,
				"title": req.Title,
				"field": buildFieldTemplate(missingNames),
			},
		}
		if typeDef != nil && typeDef.NameField != "" {
			details["name_field"] = typeDef.NameField
			details["name_field_hint"] = fmt.Sprintf("The title argument auto-populates the '%s' field", typeDef.NameField)
		}
		return nil, newError(
			ErrorRequiredField,
			fmt.Sprintf("Missing required fields: %s", strings.Join(missingNames, ", ")),
			fmt.Sprintf("Retry the same call with: field: {%s}", buildFieldTemplateExample(missingNames)),
			details,
			nil,
		)
	}

	resolvedTargetPath := pages.ResolveTargetPathWithRoots(targetPath, req.TypeName, req.Schema, req.ObjectsRoot, req.PagesRoot)
	resolvedSlugPath := pages.SlugifyPath(resolvedTargetPath)
	plannedRelPath := resolvedSlugPath
	if !strings.HasSuffix(plannedRelPath, ".md") {
		plannedRelPath += ".md"
	}
	if err := ValidateContentMutationRelPath(req.VaultConfig, plannedRelPath); err != nil {
		return nil, err
	}
	if pages.Exists(req.VaultPath, resolvedTargetPath) {
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

	validatedFields, _, err := validateCreateFieldValues(req.TypeName, fieldValues, req.Schema, nil, createRefValidationContext(req.VaultPath, req.VaultConfig))
	if err != nil {
		return nil, newError(ErrorValidationFailed, err.Error(), "Ensure values match the schema field types for this object", nil, err)
	}

	result, err := createObjectPage(createPageRequest{
		VaultPath:        req.VaultPath,
		TypeName:         req.TypeName,
		Title:            req.Title,
		TargetPath:       targetPath,
		Fields:           validatedFields,
		Schema:           req.Schema,
		TemplateOverride: templateOverride,
		TemplateDir:      req.TemplateDir,
		VaultConfig:      req.VaultConfig,
		ObjectsRoot:      req.ObjectsRoot,
		PagesRoot:        req.PagesRoot,
	})
	if err != nil {
		return nil, err
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
