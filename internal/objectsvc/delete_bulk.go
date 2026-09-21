package objectsvc

import (
	"errors"
	"fmt"
	"os"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type DeleteBulkRequest struct {
	ObjectIDs []string
	Behavior  string
	TrashDir  string
}

type DeleteBulkPreview struct {
	Action   string
	Items    []BulkPreviewItem
	Skipped  []BulkResult
	Total    int
	Behavior string
}

type DeleteBulkSummary struct {
	Action    string
	Results   []BulkResult
	Total     int
	Skipped   int
	Errors    int
	Deleted   int
	Behavior  string
	ChangeSet mutation.ChangeSet
}

func PreviewDeleteBulk(rt *vaultruntime.Runtime, req DeleteBulkRequest) (*DeleteBulkPreview, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}

	if err := rt.OpenDB(); err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open index database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}
	db := rt.DB

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	behavior, trashDir := normalizeDeletionBehavior(req.Behavior, req.TrashDir)

	for _, id := range req.ObjectIDs {
		target, err := resolveBulkDeleteTarget(rt.VaultPath, rt.VaultCfg, id)
		if err != nil {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: "object or file not found"})
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, target.FilePath); err != nil {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: err.Error()})
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
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: "file not found"})
			continue
		}

		items = append(items, BulkPreviewItem{
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

func ApplyDeleteBulk(rt *vaultruntime.Runtime, req DeleteBulkRequest) (*DeleteBulkSummary, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, err
	}

	results := make([]BulkResult, 0, len(req.ObjectIDs))
	deletedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()
	behavior, trashDir := normalizeDeletionBehavior(req.Behavior, req.TrashDir)

	for _, id := range req.ObjectIDs {
		result := BulkResult{ID: id}

		target, err := resolveBulkDeleteTarget(rt.VaultPath, rt.VaultCfg, id)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object or file not found"
			skippedCount++
			results = append(results, result)
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, target.FilePath); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}

		_, err = DeleteFile(rt, DeleteFileRequest{
			FilePath: target.FilePath,
			Behavior: behavior,
			TrashDir: trashDir,
		})
		if err != nil {
			result.Status = "error"
			var svcErr *svcerr.Error
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
