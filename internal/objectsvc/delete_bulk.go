package objectsvc

import (
	"errors"
	"fmt"
	"os"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

type DeleteBulkRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	ObjectIDs   []string
	Behavior    string
	TrashDir    string
}

type DeleteBulkPreviewItem struct {
	ID      string
	Action  string
	Details string
	Changes map[string]string
}

type DeleteBulkResult struct {
	ID     string
	Status string
	Reason string
}

type DeleteBulkPreview struct {
	Action   string
	Items    []DeleteBulkPreviewItem
	Skipped  []DeleteBulkResult
	Total    int
	Behavior string
}

type DeleteBulkSummary struct {
	Action          string
	Results         []DeleteBulkResult
	Total           int
	Skipped         int
	Errors          int
	Deleted         int
	Behavior        string
	WarningMessages []string
}

func PreviewDeleteBulk(req DeleteBulkRequest) (*DeleteBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	defer db.Close()

	items := make([]DeleteBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]DeleteBulkResult, 0)
	behavior := req.Behavior
	if behavior == "" {
		behavior = "trash"
	}
	trashDir := req.TrashDir
	if trashDir == "" {
		trashDir = ".trash"
	}

	for _, id := range req.ObjectIDs {
		objectID := req.VaultConfig.FilePathToObjectID(id)
		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, DeleteBulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}

		details := ""
		backlinks, _ := db.Backlinks(objectID)
		if len(backlinks) > 0 {
			details = fmt.Sprintf("⚠ referenced by %d objects", len(backlinks))
		}

		changes := map[string]string{"behavior": "permanent deletion"}
		if behavior == "trash" {
			changes = map[string]string{"behavior": fmt.Sprintf("move to %s/", trashDir)}
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			skipped = append(skipped, DeleteBulkResult{ID: id, Status: "skipped", Reason: "file not found"})
			continue
		}

		items = append(items, DeleteBulkPreviewItem{
			ID:      id,
			Action:  "delete",
			Details: details,
			Changes: changes,
		})
	}

	return &DeleteBulkPreview{
		Action:   "delete",
		Items:    items,
		Skipped:  skipped,
		Total:    len(req.ObjectIDs),
		Behavior: behavior,
	}, nil
}

func ApplyDeleteBulk(req DeleteBulkRequest) (*DeleteBulkSummary, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	defer db.Close()

	results := make([]DeleteBulkResult, 0, len(req.ObjectIDs))
	deletedCount := 0
	skippedCount := 0
	errorCount := 0
	warnings := make([]string, 0)
	behavior := req.Behavior
	if behavior == "" {
		behavior = "trash"
	}
	trashDir := req.TrashDir
	if trashDir == "" {
		trashDir = ".trash"
	}

	for _, id := range req.ObjectIDs {
		result := DeleteBulkResult{ID: id}

		objectID := req.VaultConfig.FilePathToObjectID(id)
		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}

		_, err = DeleteFile(DeleteFileRequest{
			VaultPath: req.VaultPath,
			FilePath:  filePath,
			Behavior:  behavior,
			TrashDir:  trashDir,
		})
		if err != nil {
			result.Status = "error"
			var svcErr *Error
			if errors.As(err, &svcErr) {
				result.Reason = svcErr.Message
			} else {
				result.Reason = fmt.Sprintf("delete failed: %v", err)
			}
			errorCount++
			results = append(results, result)
			continue
		}

		if err := db.RemoveDocument(objectID); err != nil {
			warningMsg := fmt.Sprintf("Failed to remove deleted object from index: %v", err)
			if errors.Is(err, index.ErrObjectNotFound) {
				warningMsg = "Object not found in index; consider running 'rvn reindex'"
			}
			warnings = append(warnings, warningMsg)
		}

		result.Status = "deleted"
		deletedCount++
		results = append(results, result)
	}

	return &DeleteBulkSummary{
		Action:          "delete",
		Results:         results,
		Total:           len(results),
		Skipped:         skippedCount,
		Errors:          errorCount,
		Deleted:         deletedCount,
		Behavior:        behavior,
		WarningMessages: warnings,
	}, nil
}
