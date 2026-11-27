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
	Preview   bool
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

func DeleteBulk(rt *vaultruntime.Runtime, req DeleteBulkRequest) (*DeleteBulkPreview, *DeleteBulkSummary, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, nil, err
	}

	behavior, trashDir := normalizeDeletionBehavior(req.Behavior, req.TrashDir)
	if req.Preview {
		if err := rt.OpenDB(); err != nil {
			return nil, nil, svcerr.Wrap(codes.ErrDatabase, "failed to open index database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
		}
	}

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	results := make([]BulkResult, 0, len(req.ObjectIDs))
	deletedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()

	for _, id := range req.ObjectIDs {
		target, err := resolveBulkDeleteTarget(rt.VaultPath, rt.VaultCfg, id)
		if err != nil {
			fail := BulkResult{ID: id, Status: "skipped", Reason: "object or file not found"}
			if req.Preview {
				skipped = append(skipped, fail)
			} else {
				skippedCount++
				results = append(results, fail)
			}
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, target.FilePath); err != nil {
			fail := BulkResult{ID: id, Status: bulkFailureStatus(req.Preview), Reason: err.Error()}
			if req.Preview {
				skipped = append(skipped, fail)
			} else {
				errorCount++
				results = append(results, fail)
			}
			continue
		}

		if req.Preview {
			if _, err := os.Stat(target.FilePath); os.IsNotExist(err) {
				skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: "file not found"})
				continue
			}
			details := ""
			if target.RavenObject {
				backlinks, _ := rt.DB.Backlinks(target.ObjectID)
				if len(backlinks) > 0 {
					details = fmt.Sprintf("⚠ referenced by %d objects", len(backlinks))
				}
			}
			behaviorChanges := map[string]string{"behavior": "permanent deletion"}
			if behavior == "trash" {
				behaviorChanges = map[string]string{"behavior": fmt.Sprintf("move to %s/", trashDir)}
			}
			items = append(items, BulkPreviewItem{
				ID:      id,
				Action:  "delete",
				Details: details,
				Changes: behaviorChanges,
			})
			continue
		}

		result := BulkResult{ID: id}
		_, err = deleteFile(rt, deleteFileRequest{
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

	if req.Preview {
		return &DeleteBulkPreview{
			Action:   "delete",
			Items:    items,
			Skipped:  skipped,
			Total:    len(req.ObjectIDs),
			Behavior: behavior,
		}, nil, nil
	}
	return nil, &DeleteBulkSummary{
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
