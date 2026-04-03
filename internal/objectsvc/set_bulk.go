package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

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
