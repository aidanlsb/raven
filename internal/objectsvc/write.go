package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/slugs"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type WriteRequest struct {
	TypeName    string
	Title       string
	TargetPath  string
	ReplaceBody bool
	Content     string
	FieldValues map[string]fieldvalue.FieldValue
	TemplateID  string
	CreateOnly  bool
}

type WriteResult struct {
	Status          string
	FilePath        string
	RelativePath    string
	WarningMessages []string
	ChangeSet       mutation.ChangeSet
}

func Write(rt *vaultruntime.Runtime, req WriteRequest) (*WriteResult, error) {
	if err := requireRuntime(rt); err != nil {
		return nil, err
	}
	if err := requireSchema(rt); err != nil {
		return nil, err
	}
	if req.CreateOnly {
		if strings.TrimSpace(req.TypeName) == "" {
			return nil, svcerr.New(codes.ErrInvalidInput, "type is required")
		}
		if strings.TrimSpace(req.Title) == "" {
			return nil, svcerr.New(codes.ErrInvalidInput, "title is required").WithSuggestion("Usage: rvn new <type> <title> --json")
		}
	}

	typeDef, err := lookupTypeDefinitionForCreate(rt.Schema, req.TypeName)
	if err != nil {
		return nil, err
	}

	fieldValues := normalizedCreateFieldValues(req.FieldValues, typeDef, req.Title)
	targetPath := deriveCreateTargetPath(req.TargetPath, req.Title)

	resolvedTargetPath := pages.ResolveTargetPathWithRoots(targetPath, req.TypeName, rt.Schema, objectsRoot(rt), pagesRoot(rt))
	resolvedSlugPath := slugs.PathSlug(resolvedTargetPath)
	plannedRelPath := resolvedSlugPath
	if !strings.HasSuffix(plannedRelPath, ".md") {
		plannedRelPath += ".md"
	}
	if err := mutationguard.ValidateContentMutationRelPath(rt.VaultCfg, plannedRelPath); err != nil {
		return nil, err
	}

	filePath := filepath.Join(rt.VaultPath, plannedRelPath)
	relPath := plannedRelPath
	status := "unchanged"
	var warningMessages []string

	if pages.Exists(rt.VaultPath, resolvedTargetPath) {
		if req.CreateOnly {
			return nil, svcerr.New(codes.ErrFileExists, fmt.Sprintf("file already exists: %s.md", resolvedSlugPath)).WithSuggestion("Choose a different title, or use `rvn open <reference>` to open the existing object")
		}

		originalBytes, err := os.ReadFile(filePath)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read existing object", err)
		}
		original := string(originalBytes)

		fm, err := parser.ParseFrontmatter(original)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, "failed to parse frontmatter", err).WithSuggestion("The file must have YAML frontmatter (---) for write")
		}
		if fm == nil {
			return nil, svcerr.New(codes.ErrInvalidInput, "file has no frontmatter").WithSuggestion("The file must have YAML frontmatter (---) for write")
		}
		if fm.ObjectType != "" && fm.ObjectType != req.TypeName {
			return nil, svcerr.New(codes.ErrValidationFailed, fmt.Sprintf("existing object type is '%s', cannot write as '%s'", fm.ObjectType, req.TypeName)).WithSuggestion("Choose a different title/path, or update the existing type first")
		}

		updates := make(map[string]fieldvalue.FieldValue, len(fieldValues)+1)
		if fm.ObjectType == "" {
			updates["type"] = fieldvalue.String(req.TypeName)
		}
		for key, value := range fieldValues {
			if fm.Fields != nil {
				if existing, ok := fm.Fields[key]; ok && fieldValueMatchesValue(existing, value) {
					continue
				}
			}
			updates[key] = value
		}

		nextContent := original
		if len(updates) > 0 {
			var updateWarnings []string
			refCtx := createRefValidationContext(rt)
			nextContent, updateWarnings, err = fieldmutation.PrepareValidatedFrontmatterMutationValues(
				original,
				fm,
				req.TypeName,
				updates,
				rt.Schema,
				map[string]bool{"type": true, "alias": true},
				refCtx,
			)
			if err != nil {
				return nil, err
			}
			warningMessages = append(warningMessages, updateWarnings...)
		}

		if req.ReplaceBody {
			nextContent = replaceBodyContent(nextContent, req.Content)
		}

		if nextContent != original {
			if err := atomicfile.WriteFile(filePath, []byte(nextContent), 0o644); err != nil {
				return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write updated object", err)
			}
			status = "updated"
		}
	} else {
		missingFields := requiredFieldGaps(typeDef, fieldValues)
		if len(missingFields) > 0 {
			return nil, requiredFieldsMissingError(rt, req, missingFields)
		}

		templateOverride, err := schema.ResolveTypeTemplateFile(rt.Schema, req.TypeName, req.TemplateID)
		if err != nil {
			return nil, svcerr.Wrap(codes.ErrInvalidInput, err.Error(), err).WithSuggestion("Use `rvn schema template list --type <type_name>` to see available template IDs")
		}

		allowedUnknown := map[string]bool{"type": true}
		if req.CreateOnly {
			allowedUnknown = nil
		}
		refCtx := createRefValidationContext(rt)
		validatedCreateFields, createWarnings, err := validateCreateFieldValues(
			req.TypeName,
			fieldValues,
			rt.Schema,
			allowedUnknown,
			refCtx,
		)
		if err != nil {
			if req.CreateOnly {
				return nil, svcerr.Wrap(codes.ErrValidationFailed, err.Error(), err).WithSuggestion("Ensure values match the schema field types for this object")
			}
			return nil, err
		}
		warningMessages = append(warningMessages, createWarnings...)

		createResult, err := createObjectPage(rt, createPageRequest{
			TypeName:         req.TypeName,
			Title:            req.Title,
			TargetPath:       targetPath,
			Fields:           validatedCreateFields,
			TemplateOverride: templateOverride,
		})
		if err != nil {
			return nil, err
		}
		filePath = createResult.FilePath
		relPath = createResult.RelativePath

		if req.ReplaceBody {
			createdBytes, err := os.ReadFile(filePath)
			if err != nil {
				return nil, svcerr.Wrap(codes.ErrFileRead, "failed to read created object", err)
			}
			createdContent := replaceBodyContent(string(createdBytes), req.Content)
			if createdContent != string(createdBytes) {
				if err := atomicfile.WriteFile(filePath, []byte(createdContent), 0o644); err != nil {
					return nil, svcerr.Wrap(codes.ErrFileWrite, "failed to write updated object", err)
				}
			}
		}

		status = "created"
	}

	changes := mutation.NewChangeSet()
	if status == "created" || status == "updated" {
		changes.AddChanged(relPath)
	}
	return &WriteResult{
		Status:          status,
		FilePath:        filePath,
		RelativePath:    relPath,
		WarningMessages: warningMessages,
		ChangeSet:       changes,
	}, nil
}

func requiredFieldsMissingError(rt *vaultruntime.Runtime, req WriteRequest, missingFields []requiredFieldGap) error {
	missingNames := requiredFieldGapNames(missingFields)
	msg := fmt.Sprintf("Missing required fields: %s", strings.Join(missingNames, ", "))
	details := map[string]interface{}{
		"type":  req.TypeName,
		"title": req.Title,
		"retry_with": map[string]interface{}{
			"type":  req.TypeName,
			"title": req.Title,
			"field": buildFieldTemplate(missingNames),
		},
	}
	if req.CreateOnly {
		details["missing_fields"] = requiredFieldGapDetails(missingFields)
		if typeDef, ok := rt.Schema.Types[req.TypeName]; ok && typeDef != nil && typeDef.NameField != "" {
			details["name_field"] = typeDef.NameField
			details["name_field_hint"] = fmt.Sprintf("The title argument auto-populates the '%s' field", typeDef.NameField)
		}
		return svcerr.New(codes.ErrRequiredFieldMissing, msg).
			WithSuggestion(fmt.Sprintf("Retry the same call with: field: {%s}", buildFieldTemplateExample(missingNames))).
			WithDetails(details)
	}
	details["missing_fields"] = missingNames
	return svcerr.New(codes.ErrRequiredFieldMissing, msg).
		WithSuggestion("Provide missing fields with --field").
		WithDetails(details)
}

func replaceBodyContent(fileContent, newBody string) string {
	lines := strings.Split(fileContent, "\n")

	_, endLine, ok := parser.FrontmatterBounds(lines)
	if !ok || endLine == -1 {
		return newBody
	}

	var result strings.Builder
	for i := 0; i <= endLine; i++ {
		result.WriteString(lines[i])
		result.WriteString("\n")
	}

	result.WriteString("\n")
	result.WriteString(newBody)
	if !strings.HasSuffix(newBody, "\n") {
		result.WriteString("\n")
	}

	return result.String()
}

func buildFieldTemplate(missingFields []string) map[string]interface{} {
	out := make(map[string]interface{}, len(missingFields))
	for _, field := range missingFields {
		out[field] = "<value>"
	}
	return out
}

func buildFieldTemplateExample(missingFields []string) string {
	parts := make([]string, 0, len(missingFields))
	for _, f := range missingFields {
		parts = append(parts, fmt.Sprintf(`"%s": "<value>"`, f))
	}
	return strings.Join(parts, ", ")
}
