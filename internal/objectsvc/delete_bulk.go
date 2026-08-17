package objectsvc

import (
	"errors"
	"fmt"
	"os"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteBulkRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	ObjectIDs   []string
	Behavior    string
	TrashDir    string
	Runtime     *vaultruntime.Runtime
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
	Action    string
	Results   []DeleteBulkResult
	Total     int
	Skipped   int
	Errors    int
	Deleted   int
	Behavior  string
	ChangeSet mutation.ChangeSet
}

func PreviewDeleteBulk(req DeleteBulkRequest) (*DeleteBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	rt, owned := requestRuntime(req.Runtime, req.VaultPath, req.VaultConfig, nil, nil)
	if owned {
		defer rt.Close()
	}
	if err := rt.OpenDB(); err != nil {
		return nil, newError(ErrorDatabase, "failed to open index database", "Run 'rvn reindex' to rebuild the database", nil, err)
	}
	db := rt.DB

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
		target, err := resolveBulkDeleteTarget(req.VaultPath, req.VaultConfig, id)
		if err != nil {
			skipped = append(skipped, DeleteBulkResult{ID: id, Status: "skipped", Reason: "object or file not found"})
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, target.FilePath); err != nil {
			skipped = append(skipped, DeleteBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}

		details := ""
		if target.RavenObject {
			backlinks, _ := db.Backlinks(target.ObjectID)
			if len(backlinks) > 0 {
				details = fmt.Sprintf("⚠ referenced by %d objects", len(backlinks))
			}
		}

		changes := map[string]string{"behavior": "permanent deletion"}
		if behavior == "trash" {
			changes = map[string]string{"behavior": fmt.Sprintf("move to %s/", trashDir)}
		}

		if _, err := os.Stat(target.FilePath); os.IsNotExist(err) {
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

	rt, owned := requestRuntime(req.Runtime, req.VaultPath, req.VaultConfig, nil, nil)
	if owned {
		defer rt.Close()
	}
	results := make([]DeleteBulkResult, 0, len(req.ObjectIDs))
	deletedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()
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

		target, err := resolveBulkDeleteTarget(req.VaultPath, req.VaultConfig, id)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object or file not found"
			skippedCount++
			results = append(results, result)
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, target.FilePath); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}

		_, err = DeleteFile(DeleteFileRequest{
			VaultPath: req.VaultPath,
			FilePath:  target.FilePath,
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

		result.Status = "deleted"
		deletedCount++
		changes.AddDeleted(target.RelativePath)
		results = append(results, result)
	}

	return &DeleteBulkSummary{
		Action:    "delete",
		Results:   results,
		Total:     len(results),
		Skipped:   skippedCount,
		Errors:    errorCount,
		Deleted:   deletedCount,
		Behavior:  behavior,
		ChangeSet: changes,
	}, nil
}
