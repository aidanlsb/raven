package objectsvc

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type SetBulkRequest struct {
	ObjectIDs    []string
	TypedUpdates map[string]fieldvalue.FieldValue
	Preview      bool
}

type SetBulkPreview struct {
	Action  string
	Items   []BulkPreviewItem
	Skipped []BulkResult
	Total   int
}

type SetBulkSummary struct {
	Action    string
	Results   []BulkResult
	Total     int
	Skipped   int
	Errors    int
	Modified  int
	ChangeSet mutation.ChangeSet
}

func SetBulk(rt *vaultruntime.Runtime, req SetBulkRequest) (*SetBulkPreview, *SetBulkSummary, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, nil, err
	}
	if err := requireSchema(rt); err != nil {
		return nil, nil, err
	}

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	results := make([]BulkResult, 0, len(req.ObjectIDs))
	modifiedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()

	for _, id := range req.ObjectIDs {
		if strings.Contains(id, "#") {
			fail := BulkResult{ID: id, Status: "skipped", Reason: "set only supports file-level object frontmatter"}
			if req.Preview {
				skipped = append(skipped, fail)
			} else {
				skippedCount++
				results = append(results, fail)
			}
			continue
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(rt.VaultPath, id, rt.VaultCfg)
		if err != nil {
			fail := BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
			if req.Preview {
				skipped = append(skipped, fail)
			} else {
				skippedCount++
				results = append(results, fail)
			}
			continue
		}

		fileResult, setErr := setObjectFile(rt, setObjectFileRequest{
			FilePath:      filePath,
			ObjectID:      id,
			TypedUpdates:  req.TypedUpdates,
			AllowedFields: map[string]bool{"alias": true},
			Preview:       req.Preview,
		})
		if setErr != nil {
			fail := BulkResult{ID: id, Status: bulkFailureStatus(req.Preview), Reason: setBulkReasonFromError(setErr)}
			if req.Preview {
				skipped = append(skipped, fail)
			} else {
				countBulkFailure(false, &skippedCount, &errorCount)
				results = append(results, fail)
			}
			continue
		}

		if req.Preview {
			items = append(items, BulkPreviewItem{
				ID:      id,
				Action:  "set",
				Changes: formatSetPreviewChanges(fileResult.PreviousFields, fileResult.ResolvedUpdates),
			})
			continue
		}

		modifiedCount++
		if relPath, relErr := filepath.Rel(rt.VaultPath, filePath); relErr == nil {
			changes.AddChanged(relPath)
		}
		results = append(results, BulkResult{ID: id, Status: "modified"})
	}

	if req.Preview {
		return &SetBulkPreview{
			Action:  "set",
			Items:   items,
			Skipped: skipped,
			Total:   len(req.ObjectIDs),
		}, nil, nil
	}
	return nil, &SetBulkSummary{
		Action:    "set",
		Results:   results,
		Total:     len(results),
		Skipped:   skippedCount,
		Errors:    errorCount,
		Modified:  modifiedCount,
		ChangeSet: changes,
	}, nil
}

func setBulkReasonFromError(err error) string {
	var svcErr *svcerr.Error
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

func formatSetPreviewChanges(existingFields map[string]fieldvalue.FieldValue, resolvedUpdates map[string]string) map[string]string {
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

func previewFieldValue(value fieldvalue.FieldValue) string {
	if value.IsNull() {
		return "null"
	}
	return fieldmutation.SerializeFieldValueLiteral(value)
}
