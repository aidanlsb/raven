package objectsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

type ErrorCode string

const (
	ErrorTypeNotFound     ErrorCode = "TYPE_NOT_FOUND"
	ErrorRequiredField    ErrorCode = "REQUIRED_FIELD"
	ErrorInvalidInput     ErrorCode = "INVALID_INPUT"
	ErrorFileNotFound     ErrorCode = "FILE_NOT_FOUND"
	ErrorFileExists       ErrorCode = "FILE_EXISTS"
	ErrorRefNotFound      ErrorCode = "REF_NOT_FOUND"
	ErrorRefAmbiguous     ErrorCode = "REF_AMBIGUOUS"
	ErrorDatabase         ErrorCode = "DATABASE"
	ErrorValidationFailed ErrorCode = "VALIDATION_FAILED"
	ErrorFileRead         ErrorCode = "FILE_READ"
	ErrorFileWrite        ErrorCode = "FILE_WRITE"
	ErrorUnexpected       ErrorCode = "UNEXPECTED"
)

type Error struct {
	Code       ErrorCode
	Message    string
	Suggestion string
	Details    map[string]interface{}
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newError(code ErrorCode, message, suggestion string, details map[string]interface{}, cause error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
		Details:    details,
		Cause:      cause,
	}
}

type UpsertRequest struct {
	VaultPath   string
	TypeName    string
	Title       string
	TargetPath  string
	ReplaceBody bool
	Content     string
	FieldValues map[string]schema.FieldValue
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	ObjectsRoot string
	PagesRoot   string
	TemplateDir string
}

type UpsertResult struct {
	Status          string
	FilePath        string
	RelativePath    string
	WarningMessages []string
}

func Upsert(req UpsertRequest) (*UpsertResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}

	fieldValues := cloneFieldValues(req.FieldValues)

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

	ensureNameFieldValue(fieldValues, typeDef, req.Title)

	slugified := pages.SlugifyPath(
		pages.ResolveTargetPathWithRoots(req.TargetPath, req.TypeName, req.Schema, req.ObjectsRoot, req.PagesRoot),
	)
	if !strings.HasSuffix(slugified, ".md") {
		slugified += ".md"
	}
	if err := ValidateContentMutationRelPath(req.VaultConfig, slugified); err != nil {
		return nil, err
	}

	filePath := filepath.Join(req.VaultPath, slugified)
	relPath := slugified
	status := "unchanged"
	var warningMessages []string

	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		missingRequired := requiredFieldGapsValues(typeDef, fieldValues)
		if len(missingRequired) > 0 {
			msg := fmt.Sprintf("Missing required fields: %s", strings.Join(missingRequired, ", "))
			return nil, newError(
				ErrorRequiredField,
				msg,
				"Provide missing fields with --field",
				map[string]interface{}{
					"type":           req.TypeName,
					"title":          req.Title,
					"missing_fields": missingRequired,
					"retry_with": map[string]interface{}{
						"type":  req.TypeName,
						"title": req.Title,
						"field": buildFieldTemplate(missingRequired),
					},
				},
				nil,
			)
		}

		validatedCreateFields, createWarnings, err := fieldmutation.PrepareValidatedFieldMutationValues(
			req.TypeName,
			nil,
			fieldValues,
			req.Schema,
			map[string]bool{"type": true},
			&fieldmutation.RefValidationContext{
				VaultPath:   req.VaultPath,
				VaultConfig: req.VaultConfig,
			},
		)
		if err != nil {
			return nil, err
		}
		warningMessages = append(warningMessages, createWarnings...)

		createResult, err := pages.Create(pages.CreateOptions{
			VaultPath:   req.VaultPath,
			TypeName:    req.TypeName,
			Title:       req.Title,
			TargetPath:  req.TargetPath,
			Fields:      validatedCreateFields,
			Schema:      req.Schema,
			TemplateDir: req.TemplateDir,
			ProtectedPrefixes: func() []string {
				if req.VaultConfig == nil {
					return nil
				}
				return req.VaultConfig.ProtectedPrefixes
			}(),
			ObjectsRoot: req.ObjectsRoot,
			PagesRoot:   req.PagesRoot,
		})
		if err != nil {
			return nil, newError(ErrorFileWrite, "failed to create object", "", nil, err)
		}
		filePath = createResult.FilePath
		relPath = createResult.RelativePath

		if req.ReplaceBody {
			createdBytes, err := os.ReadFile(filePath)
			if err != nil {
				return nil, newError(ErrorFileRead, "failed to read created object", "", nil, err)
			}
			createdContent := replaceBodyContent(string(createdBytes), req.Content)
			if createdContent != string(createdBytes) {
				if err := atomicfile.WriteFile(filePath, []byte(createdContent), 0o644); err != nil {
					return nil, newError(ErrorFileWrite, "failed to write updated object", "", nil, err)
				}
			}
		}

		status = "created"
	} else if err != nil {
		return nil, newError(ErrorFileRead, "failed to inspect existing object", "", nil, err)
	} else {
		originalBytes, err := os.ReadFile(filePath)
		if err != nil {
			return nil, newError(ErrorFileRead, "failed to read existing object", "", nil, err)
		}
		original := string(originalBytes)

		fm, err := parser.ParseFrontmatter(original)
		if err != nil {
			return nil, newError(ErrorInvalidInput, "failed to parse frontmatter", "The file must have YAML frontmatter (---) for upsert", nil, err)
		}
		if fm == nil {
			return nil, newError(
				ErrorInvalidInput,
				"file has no frontmatter",
				"The file must have YAML frontmatter (---) for upsert",
				nil,
				nil,
			)
		}
		if fm.ObjectType != "" && fm.ObjectType != req.TypeName {
			return nil, newError(
				ErrorValidationFailed,
				fmt.Sprintf("existing object type is '%s', cannot upsert as '%s'", fm.ObjectType, req.TypeName),
				"Choose a different title/path, or update the existing type first",
				nil,
				nil,
			)
		}

		updates := make(map[string]schema.FieldValue, len(fieldValues)+1)
		if fm.ObjectType == "" {
			updates["type"] = schema.String(req.TypeName)
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
			nextContent, updateWarnings, err = fieldmutation.PrepareValidatedFrontmatterMutationValues(
				original,
				fm,
				req.TypeName,
				updates,
				req.Schema,
				map[string]bool{"type": true, "alias": true},
				&fieldmutation.RefValidationContext{
					VaultPath:   req.VaultPath,
					VaultConfig: req.VaultConfig,
				},
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
				return nil, newError(ErrorFileWrite, "failed to write updated object", "", nil, err)
			}
			status = "updated"
		}
	}

	return &UpsertResult{
		Status:          status,
		FilePath:        filePath,
		RelativePath:    relPath,
		WarningMessages: warningMessages,
	}, nil
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
