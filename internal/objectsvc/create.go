package objectsvc

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type CreateRequest struct {
	VaultPath   string
	TypeName    string
	Title       string
	TargetPath  string
	FieldValues map[string]fieldvalue.FieldValue
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	ObjectsRoot string
	PagesRoot   string
	TemplateDir string
	TemplateID  string
	Runtime     *vaultruntime.Runtime
}

type CreateResult struct {
	FilePath     string
	RelativePath string
	ChangeSet    mutation.ChangeSet
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
	rt, owned := requestRuntime(req.Runtime, req.VaultPath, req.VaultConfig, req.Schema, nil)
	if owned {
		defer rt.Close()
	}

	typeDef, err := lookupTypeDefinitionForCreate(req.Schema, req.TypeName)
	if err != nil {
		return nil, err
	}

	targetPath := deriveCreateTargetPath(req.TargetPath, req.Title)

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
	resolvedSlugPath := slugs.PathSlug(resolvedTargetPath)
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

	refCtx, err := createRefValidationContext(rt, req.TypeName, nil)
	if err != nil {
		return nil, newError(ErrorValidationFailed, err.Error(), "Ensure values match the schema field types for this object", nil, err)
	}
	validatedFields, _, err := validateCreateFieldValues(req.TypeName, fieldValues, req.Schema, nil, refCtx)
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

	changes := mutation.NewChangeSet()
	changes.AddChanged(result.RelativePath)
	return &CreateResult{
		FilePath:     result.FilePath,
		RelativePath: result.RelativePath,
		ChangeSet:    changes,
	}, nil
}

func buildFieldTemplateExample(missingFields []string) string {
	parts := make([]string, 0, len(missingFields))
	for _, f := range missingFields {
		parts = append(parts, fmt.Sprintf(`"%s": "<value>"`, f))
	}
	return strings.Join(parts, ", ")
}
