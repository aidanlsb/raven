package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

type DeleteFileRequest struct {
	VaultPath string
	FilePath  string
	Behavior  string
	TrashDir  string
	Now       func() time.Time
}

type DeleteFileResult struct {
	Behavior  string
	TrashPath string
}

func DeleteFile(req DeleteFileRequest) (*DeleteFileResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if strings.TrimSpace(req.FilePath) == "" {
		return nil, newError(ErrorInvalidInput, "file path is required", "", nil, nil)
	}

	behavior := strings.TrimSpace(req.Behavior)
	if behavior == "" {
		behavior = "trash"
	}

	switch behavior {
	case "trash":
		trashDir := strings.TrimSpace(req.TrashDir)
		if trashDir == "" {
			trashDir = ".trash"
		}

		trashRoot := filepath.Join(req.VaultPath, trashDir)
		if err := os.MkdirAll(trashRoot, 0o755); err != nil {
			return nil, newError(ErrorFileWrite, "failed to create trash directory", "", nil, err)
		}

		relPath, err := filepath.Rel(req.VaultPath, req.FilePath)
		if err != nil {
			return nil, newError(ErrorInvalidInput, "failed to compute relative path", "", nil, err)
		}
		destPath := filepath.Join(trashRoot, relPath)

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return nil, newError(ErrorFileWrite, "failed to create trash parent directory", "", nil, err)
		}

		if _, err := os.Stat(destPath); err == nil {
			nowFn := req.Now
			if nowFn == nil {
				nowFn = time.Now
			}
			timestamp := nowFn().Format("2006-01-02-150405")
			ext := filepath.Ext(destPath)
			base := strings.TrimSuffix(filepath.Base(destPath), ext)
			destPath = filepath.Join(filepath.Dir(destPath), fmt.Sprintf("%s-%s%s", base, timestamp, ext))
		}

		if err := os.Rename(req.FilePath, destPath); err != nil {
			return nil, newError(ErrorFileWrite, "failed to move file to trash", "", nil, err)
		}

		return &DeleteFileResult{
			Behavior:  behavior,
			TrashPath: destPath,
		}, nil

	case "permanent":
		if err := os.Remove(req.FilePath); err != nil {
			return nil, newError(ErrorFileWrite, "failed to delete file", "", nil, err)
		}
		return &DeleteFileResult{Behavior: behavior}, nil

	default:
		return nil, newError(ErrorInvalidInput, fmt.Sprintf("invalid deletion behavior: %s", behavior), "Use 'trash' or 'permanent'", nil, nil)
	}
}

// --- single-object delete by reference ---

type DeleteByReferenceRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	Reference   string
	Behavior    string
	TrashDir    string
}

type DeleteByReferenceResult struct {
	ObjectID        string
	Behavior        string
	TrashPath       string
	Backlinks       []model.Reference
	WarningMessages []string
}

func PreviewDeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	if strings.TrimSpace(req.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if strings.TrimSpace(req.Reference) == "" {
		return nil, newError(ErrorInvalidInput, "reference is required", "Usage: rvn delete <object-id>", nil, nil)
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	db, err := index.Open(req.VaultPath)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	defer db.Close()
	db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())

	backlinks, err := db.Backlinks(resolved.ObjectID)
	if err != nil {
		return nil, newError(ErrorDatabase, "failed to read backlinks", "Run 'rvn reindex' to rebuild the database", nil, err)
	}

	return &DeleteByReferenceResult{
		ObjectID:  resolved.ObjectID,
		Behavior:  req.Behavior,
		Backlinks: backlinks,
	}, nil
}

func DeleteByReference(req DeleteByReferenceRequest) (*DeleteByReferenceResult, error) {
	preview, err := PreviewDeleteByReference(req)
	if err != nil {
		return nil, err
	}

	resolved, err := resolveReferenceForMutation(req.VaultPath, req.VaultConfig, req.Schema, req.Reference)
	if err != nil {
		return nil, err
	}

	delResult, err := DeleteFile(DeleteFileRequest{
		VaultPath: req.VaultPath,
		FilePath:  resolved.FilePath,
		Behavior:  req.Behavior,
		TrashDir:  req.TrashDir,
	})
	if err != nil {
		return nil, err
	}

	warnings := make([]string, 0)
	db, err := index.Open(req.VaultPath)
	if err != nil {
		warnings = append(warnings, fmt.Sprintf("Failed to open index database while removing deleted object: %v", err))
	} else {
		defer db.Close()
		db.SetDailyDirectory(req.VaultConfig.GetDailyDirectory())
		if err := db.RemoveDocument(preview.ObjectID); err != nil {
			if errors.Is(err, index.ErrObjectNotFound) {
				warnings = append(warnings, "Object not found in index; consider running 'rvn reindex'")
			} else {
				warnings = append(warnings, fmt.Sprintf("Failed to remove deleted object from index: %v", err))
			}
		}
	}

	return &DeleteByReferenceResult{
		ObjectID:        preview.ObjectID,
		Behavior:        delResult.Behavior,
		TrashPath:       delResult.TrashPath,
		Backlinks:       preview.Backlinks,
		WarningMessages: warnings,
	}, nil
}

// --- bulk delete ---

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
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
			skipped = append(skipped, DeleteBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
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
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
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
