package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/atomicfile"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

// --- file-level set ---

type SetObjectFileRequest struct {
	VaultPath     string
	VaultConfig   *config.VaultConfig
	FilePath      string
	ObjectID      string
	TypedUpdates  map[string]schema.FieldValue
	Schema        *schema.Schema
	AllowedFields map[string]bool
	ParseOptions  *parser.ParseOptions
}

type SetObjectFileResult struct {
	ObjectID        string
	ObjectType      string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetObjectFile(req SetObjectFileRequest) (*SetObjectFileResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.FilePath); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read file", "", nil, err)
	}

	fm, err := parser.ParseFrontmatter(string(content))
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse frontmatter", "Failed to parse frontmatter", nil, err)
	}
	if fm == nil {
		return nil, newError(ErrorInvalidInput, "file has no frontmatter", "The file must have YAML frontmatter (---) to set fields", nil, nil)
	}

	objectType := fm.ObjectType
	if objectType == "" {
		objectType = "page"
	}

	newContent, warningMessages, err := fieldmutation.PrepareValidatedFrontmatterMutationValues(
		string(content),
		fm,
		objectType,
		req.TypedUpdates,
		req.Schema,
		req.AllowedFields,
		&fieldmutation.RefValidationContext{
			VaultPath:    req.VaultPath,
			VaultConfig:  req.VaultConfig,
			ParseOptions: req.ParseOptions,
		},
	)
	if err != nil {
		return nil, err
	}

	updatedFM, err := parser.ParseFrontmatter(newContent)
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse updated frontmatter", "", nil, err)
	}
	if updatedFM == nil {
		return nil, newError(ErrorInvalidInput, "file has no frontmatter after update", "", nil, nil)
	}

	resolvedUpdates := make(map[string]string, len(req.TypedUpdates))
	for key := range req.TypedUpdates {
		resolvedUpdates[key] = fieldmutation.SerializeFieldValueLiteral(updatedFM.Fields[key])
	}

	if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
		return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
	}

	previousFields := make(map[string]schema.FieldValue, len(fm.Fields))
	for key, value := range fm.Fields {
		previousFields[key] = value
	}

	return &SetObjectFileResult{
		ObjectID:        req.ObjectID,
		ObjectType:      objectType,
		ResolvedUpdates: resolvedUpdates,
		WarningMessages: warningMessages,
		PreviousFields:  previousFields,
	}, nil
}

// --- embedded-object set ---

type SetEmbeddedObjectRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	FilePath       string
	ObjectID       string
	TypedUpdates   map[string]schema.FieldValue
	Schema         *schema.Schema
	AllowedFields  map[string]bool
	DocumentParser *parser.ParseOptions
}

type SetEmbeddedObjectResult struct {
	ObjectID        string
	ObjectType      string
	Slug            string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetEmbeddedObject(req SetEmbeddedObjectRequest) (*SetEmbeddedObjectResult, error) {
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, req.FilePath); err != nil {
		return nil, err
	}

	_, slug, isEmbedded := paths.ParseEmbeddedID(req.ObjectID)
	if !isEmbedded {
		return nil, newError(ErrorInvalidInput, "invalid embedded object ID", "Expected format: file-id#embedded-id", nil, nil)
	}

	contentBytes, err := os.ReadFile(req.FilePath)
	if err != nil {
		return nil, newError(ErrorFileRead, "failed to read file", "", nil, err)
	}
	content := string(contentBytes)

	doc, err := parser.ParseDocumentWithOptions(content, req.FilePath, req.VaultPath, req.DocumentParser)
	if err != nil {
		return nil, newError(ErrorInvalidInput, "failed to parse document", "Failed to parse document", nil, err)
	}

	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == req.ObjectID {
			targetObj = obj
			break
		}
	}
	if targetObj == nil {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("embedded object '%s' not found in file", slug),
			"Check that the embedded ID exists in the file",
			nil,
			nil,
		)
	}
	if targetObj.ParentID == nil {
		return nil, newError(
			ErrorInvalidInput,
			"cannot use embedded set on file-level object",
			"Use 'rvn set <file-id> field=value' instead",
			nil,
			nil,
		)
	}

	typeDeclLine := targetObj.LineStart + 1
	lines := strings.Split(content, "\n")
	if typeDeclLine-1 >= len(lines) {
		return nil, newError(ErrorInvalidInput, "type declaration line not found", "", nil, nil)
	}

	declLine := lines[typeDeclLine-1]
	if !strings.HasPrefix(strings.TrimSpace(declLine), "::") {
		return nil, newError(
			ErrorInvalidInput,
			fmt.Sprintf("expected type declaration at line %d, found: %s", typeDeclLine, strings.TrimSpace(declLine)),
			"The embedded object may have been modified or is in an unexpected format",
			nil,
			nil,
		)
	}

	validatedUpdates, warningMessages, err := fieldmutation.PrepareValidatedFieldMutationValues(
		targetObj.ObjectType,
		targetObj.Fields,
		req.TypedUpdates,
		req.Schema,
		req.AllowedFields,
		&fieldmutation.RefValidationContext{
			VaultPath:    req.VaultPath,
			VaultConfig:  req.VaultConfig,
			ParseOptions: req.DocumentParser,
		},
	)
	if err != nil {
		return nil, err
	}

	newFields := make(map[string]schema.FieldValue, len(targetObj.Fields)+len(validatedUpdates))
	for key, value := range targetObj.Fields {
		newFields[key] = value
	}
	for key, value := range validatedUpdates {
		newFields[key] = value
	}

	leadingSpace := ""
	for _, c := range declLine {
		if c == ' ' || c == '\t' {
			leadingSpace += string(c)
			continue
		}
		break
	}
	lines[typeDeclLine-1] = leadingSpace + parser.SerializeTypeDeclaration(targetObj.ObjectType, newFields)

	newContent := strings.Join(lines, "\n")
	if err := atomicfile.WriteFile(req.FilePath, []byte(newContent), 0o644); err != nil {
		return nil, newError(ErrorFileWrite, "failed to write file", "", nil, err)
	}

	previousFields := make(map[string]schema.FieldValue, len(targetObj.Fields))
	for key, value := range targetObj.Fields {
		previousFields[key] = value
	}

	return &SetEmbeddedObjectResult{
		ObjectID:        req.ObjectID,
		ObjectType:      targetObj.ObjectType,
		Slug:            slug,
		ResolvedUpdates: fieldmutation.SerializeFieldValueMap(validatedUpdates),
		WarningMessages: warningMessages,
		PreviousFields:  previousFields,
	}, nil
}

// --- single-object set by reference ---

type SetByReferenceRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	Schema       *schema.Schema
	Reference    string
	TypedUpdates map[string]schema.FieldValue
	ParseOptions *parser.ParseOptions
}

type SetByReferenceResult struct {
	FilePath        string
	RelativePath    string
	ObjectID        string
	ObjectType      string
	Embedded        bool
	EmbeddedSlug    string
	ResolvedUpdates map[string]string
	WarningMessages []string
	PreviousFields  map[string]schema.FieldValue
}

func SetByReference(req SetByReferenceRequest) (*SetByReferenceResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn set <object-id> field=value...", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	if resolved.IsSection {
		result, err := SetEmbeddedObject(SetEmbeddedObjectRequest{
			VaultPath:      req.VaultPath,
			VaultConfig:    req.VaultConfig,
			FilePath:       resolved.FilePath,
			ObjectID:       resolved.ObjectID,
			TypedUpdates:   req.TypedUpdates,
			Schema:         req.Schema,
			AllowedFields:  map[string]bool{"alias": true, "id": true},
			DocumentParser: req.ParseOptions,
		})
		if err != nil {
			return nil, err
		}
		relPath, _ := filepath.Rel(req.VaultPath, resolved.FilePath)
		relPath = filepath.ToSlash(relPath)
		_, slug, _ := paths.ParseEmbeddedID(resolved.ObjectID)
		return &SetByReferenceResult{
			FilePath:        resolved.FilePath,
			RelativePath:    relPath,
			ObjectID:        resolved.ObjectID,
			ObjectType:      result.ObjectType,
			Embedded:        true,
			EmbeddedSlug:    slug,
			ResolvedUpdates: result.ResolvedUpdates,
			WarningMessages: result.WarningMessages,
			PreviousFields:  result.PreviousFields,
		}, nil
	}

	result, err := SetObjectFile(SetObjectFileRequest{
		VaultPath:     req.VaultPath,
		VaultConfig:   req.VaultConfig,
		FilePath:      resolved.FilePath,
		ObjectID:      resolved.ObjectID,
		TypedUpdates:  req.TypedUpdates,
		Schema:        req.Schema,
		AllowedFields: map[string]bool{"alias": true},
		ParseOptions:  req.ParseOptions,
	})
	if err != nil {
		return nil, err
	}

	relPath, _ := filepath.Rel(req.VaultPath, resolved.FilePath)
	relPath = filepath.ToSlash(relPath)
	return &SetByReferenceResult{
		FilePath:        resolved.FilePath,
		RelativePath:    relPath,
		ObjectID:        resolved.ObjectID,
		ObjectType:      result.ObjectType,
		Embedded:        false,
		ResolvedUpdates: result.ResolvedUpdates,
		WarningMessages: result.WarningMessages,
		PreviousFields:  result.PreviousFields,
	}, nil
}

// --- bulk set ---

type SetBulkRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	Schema       *schema.Schema
	ObjectIDs    []string
	TypedUpdates map[string]schema.FieldValue
	ParseOptions *parser.ParseOptions
}

type SetBulkPreviewItem struct {
	ID      string
	Action  string
	Changes map[string]string
}

type SetBulkResult struct {
	ID     string
	Status string
	Reason string
}

type SetBulkPreview struct {
	Action  string
	Items   []SetBulkPreviewItem
	Skipped []SetBulkResult
	Total   int
}

type SetBulkSummary struct {
	Action   string
	Results  []SetBulkResult
	Total    int
	Skipped  int
	Errors   int
	Modified int
}

func PreviewSetBulk(req SetBulkRequest) (*SetBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}

	items := make([]SetBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]SetBulkResult, 0)

	for _, id := range req.ObjectIDs {
		if strings.Contains(id, "#") {
			item, skip := previewSetBulkEmbedded(req, id)
			if skip != nil {
				skipped = append(skipped, *skip)
				continue
			}
			if item != nil {
				items = append(items, *item)
			}
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
			skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}

		content, err := os.ReadFile(filePath)
		if err != nil {
			skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)})
			continue
		}

		fm, err := parser.ParseFrontmatter(string(content))
		if err != nil || fm == nil {
			skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: "no frontmatter"})
			continue
		}

		objectType := fm.ObjectType
		if objectType == "" {
			objectType = "page"
		}
		validatedUpdates, _, err := fieldmutation.PrepareValidatedFieldMutationValues(
			objectType,
			fm.Fields,
			req.TypedUpdates,
			req.Schema,
			map[string]bool{"alias": true},
			&fieldmutation.RefValidationContext{
				VaultPath:    req.VaultPath,
				VaultConfig:  req.VaultConfig,
				ParseOptions: req.ParseOptions,
			},
		)
		if err != nil {
			var validationErr *fieldmutation.ValidationError
			if errors.As(err, &validationErr) {
				skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: validationErr.Error()})
			} else {
				skipped = append(skipped, SetBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("validation error: %v", err)})
			}
			continue
		}

		items = append(items, SetBulkPreviewItem{
			ID:      id,
			Action:  "set",
			Changes: formatSetPreviewChanges(fm.Fields, validatedUpdates),
		})
	}

	return &SetBulkPreview{
		Action:  "set",
		Items:   items,
		Skipped: skipped,
		Total:   len(req.ObjectIDs),
	}, nil
}

func ApplySetBulk(req SetBulkRequest, onModified func(filePath string)) (*SetBulkSummary, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return nil, newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}

	results := make([]SetBulkResult, 0, len(req.ObjectIDs))
	modifiedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, id := range req.ObjectIDs {
		result := SetBulkResult{ID: id}

		if strings.Contains(id, "#") {
			filePath, err := applySetBulkEmbedded(req, id)
			if err != nil {
				result.Status = "error"
				result.Reason = err.Error()
				errorCount++
			} else {
				result.Status = "modified"
				modifiedCount++
				if onModified != nil {
					onModified(filePath)
				}
			}
			results = append(results, result)
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}

		_, err = SetObjectFile(SetObjectFileRequest{
			VaultPath:     req.VaultPath,
			VaultConfig:   req.VaultConfig,
			FilePath:      filePath,
			ObjectID:      id,
			TypedUpdates:  req.TypedUpdates,
			Schema:        req.Schema,
			AllowedFields: map[string]bool{"alias": true},
			ParseOptions:  req.ParseOptions,
		})
		if err != nil {
			result.Status = "error"
			result.Reason = setBulkReasonFromError(err)
			errorCount++
			results = append(results, result)
			continue
		}

		result.Status = "modified"
		modifiedCount++
		if onModified != nil {
			onModified(filePath)
		}
		results = append(results, result)
	}

	return &SetBulkSummary{
		Action:   "set",
		Results:  results,
		Total:    len(results),
		Skipped:  skippedCount,
		Errors:   errorCount,
		Modified: modifiedCount,
	}, nil
}

func previewSetBulkEmbedded(req SetBulkRequest, id string) (*SetBulkPreviewItem, *SetBulkResult) {
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: "invalid embedded ID format"}
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, fileID, req.VaultConfig)
	if err != nil {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: "parent file not found"}
	}
	if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: err.Error()}
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)}
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, req.VaultPath, req.ParseOptions)
	if err != nil {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("parse error: %v", err)}
	}

	var targetObj *parser.ParsedObject
	for _, obj := range doc.Objects {
		if obj.ID == id {
			targetObj = obj
			break
		}
	}
	if targetObj == nil {
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: "embedded object not found"}
	}
	validatedUpdates, _, err := fieldmutation.PrepareValidatedFieldMutationValues(
		targetObj.ObjectType,
		targetObj.Fields,
		req.TypedUpdates,
		req.Schema,
		map[string]bool{"alias": true, "id": true},
		&fieldmutation.RefValidationContext{
			VaultPath:    req.VaultPath,
			VaultConfig:  req.VaultConfig,
			ParseOptions: req.ParseOptions,
		},
	)
	if err != nil {
		var validationErr *fieldmutation.ValidationError
		if errors.As(err, &validationErr) {
			return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: validationErr.Error()}
		}
		return nil, &SetBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("validation error: %v", err)}
	}

	return &SetBulkPreviewItem{
		ID:      id,
		Action:  "set",
		Changes: formatSetPreviewChanges(targetObj.Fields, validatedUpdates),
	}, nil
}

func applySetBulkEmbedded(req SetBulkRequest, id string) (string, error) {
	fileID, _, isEmbedded := paths.ParseEmbeddedID(id)
	if !isEmbedded {
		return "", fmt.Errorf("invalid embedded ID format")
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, fileID, req.VaultConfig)
	if err != nil {
		return "", fmt.Errorf("parent file not found: %w", err)
	}

	_, err = SetEmbeddedObject(SetEmbeddedObjectRequest{
		VaultPath:      req.VaultPath,
		VaultConfig:    req.VaultConfig,
		FilePath:       filePath,
		ObjectID:       id,
		TypedUpdates:   req.TypedUpdates,
		Schema:         req.Schema,
		AllowedFields:  map[string]bool{"alias": true, "id": true},
		DocumentParser: req.ParseOptions,
	})
	if err != nil {
		var svcErr *Error
		if errors.As(err, &svcErr) {
			return "", errors.New(svcErr.Message)
		}
		return "", err
	}

	return filePath, nil
}

func setBulkReasonFromError(err error) string {
	var svcErr *Error
	var unknownErr *fieldmutation.UnknownFieldMutationError
	var validationErr *fieldmutation.ValidationError

	switch {
	case errors.As(err, &svcErr):
		return svcErr.Message
	case errors.As(err, &unknownErr):
		return unknownErr.Error()
	case errors.As(err, &validationErr):
		return validationErr.Error()
	default:
		return fmt.Sprintf("update error: %v", err)
	}
}

func formatSetPreviewChanges(existingFields map[string]schema.FieldValue, updates map[string]schema.FieldValue) map[string]string {
	resolvedUpdates := fieldmutation.SerializeFieldValueMap(updates)
	changes := make(map[string]string, len(resolvedUpdates))
	for field, resolvedVal := range resolvedUpdates {
		oldVal := "<unset>"
		if existingFields != nil {
			if value, ok := existingFields[field]; ok {
				oldVal = previewFieldValue(value)
			}
		}
		changes[field] = fmt.Sprintf("%s (was: %s)", resolvedVal, oldVal)
	}
	return changes
}

func previewFieldValue(value schema.FieldValue) string {
	if value.IsNull() {
		return "null"
	}
	return fieldmutation.SerializeFieldValueLiteral(value)
}
