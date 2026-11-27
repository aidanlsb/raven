package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type MoveBulkRequest struct {
	ObjectIDs      []string
	DestinationDir string
	UpdateRefs     bool
	Preview        bool
}

type MoveBulkPreview struct {
	Action      string
	Items       []BulkPreviewItem
	Skipped     []BulkResult
	Total       int
	Destination string
}

type MoveBulkSummary struct {
	Action          string
	Results         []BulkResult
	Total           int
	Skipped         int
	Errors          int
	Moved           int
	Destination     string
	WarningMessages []string
	ChangeSet       mutation.ChangeSet
}

func MoveBulk(rt *vaultruntime.Runtime, req MoveBulkRequest) (*MoveBulkPreview, *MoveBulkSummary, error) {
	if err := requireVaultConfig(rt); err != nil {
		return nil, nil, err
	}
	if !strings.HasSuffix(req.DestinationDir, "/") {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "destination must be a directory (end with /)").WithSuggestion("Example: rvn move --stdin archive/projects/")
	}
	if sectionIDs := moveBulkSectionIDs(req.ObjectIDs); len(sectionIDs) > 0 {
		return nil, nil, svcerr.New(codes.ErrInvalidInput, "rvn move does not accept section sources").WithSuggestion(`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`).WithDetails(map[string]interface{}{"section_ids": sectionIDs})
	}

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	results := make([]BulkResult, 0, len(req.ObjectIDs))
	movedCount := 0
	skippedCount := 0
	errorCount := 0
	warnings := make([]string, 0)
	changes := mutation.NewChangeSet()

	for _, id := range req.ObjectIDs {
		sourceFile, destPath, fullDestPath, fail := planMoveBulkItem(rt, req, id)
		if fail != nil {
			if req.Preview {
				skipped = append(skipped, *fail)
			} else {
				if fail.Status == "skipped" {
					skippedCount++
				} else {
					errorCount++
				}
				results = append(results, *fail)
			}
			continue
		}

		if req.Preview {
			items = append(items, BulkPreviewItem{
				ID:      id,
				Action:  "move",
				Details: fmt.Sprintf("→ %s", destPath),
			})
			continue
		}

		result := BulkResult{ID: id}
		relSource, _ := filepath.Rel(rt.VaultPath, sourceFile)
		sourceID := rt.VaultCfg.FilePathToObjectID(relSource)
		destID := rt.VaultCfg.FilePathToObjectID(destPath)

		serviceResult, err := MoveFile(rt, MoveFileRequest{
			SourceFile:        sourceFile,
			DestinationFile:   fullDestPath,
			SourceObjectID:    sourceID,
			DestinationObject: destID,
			UpdateRefs:        req.UpdateRefs,
			PriorMoves:        append([]mutation.Move(nil), changes.Moved...),
		})
		if err != nil {
			result.Status = "error"
			var svcErr *svcerr.Error
			if errors.As(err, &svcErr) {
				result.Reason = svcErr.Message
			} else {
				result.Reason = fmt.Sprintf("move failed: %v", err)
			}
			errorCount++
			results = append(results, result)
			continue
		}

		warnings = append(warnings, serviceResult.WarningMessages...)
		changes.Merge(serviceResult.ChangeSet)
		result.Status = "moved"
		result.Details = destPath
		movedCount++
		results = append(results, result)
	}

	if req.Preview {
		return &MoveBulkPreview{
			Action:      "move",
			Items:       items,
			Skipped:     skipped,
			Total:       len(req.ObjectIDs),
			Destination: req.DestinationDir,
		}, nil, nil
	}
	return nil, &MoveBulkSummary{
		Action:          "move",
		Results:         results,
		Total:           len(results),
		Skipped:         skippedCount,
		Errors:          errorCount,
		Moved:           movedCount,
		Destination:     req.DestinationDir,
		WarningMessages: warnings,
		ChangeSet:       changes,
	}, nil
}

func planMoveBulkItem(rt *vaultruntime.Runtime, req MoveBulkRequest, id string) (sourceFile, destPath, fullDestPath string, fail *BulkResult) {
	sourceFile, err := vault.ResolveObjectToFileWithConfig(rt.VaultPath, id, rt.VaultCfg)
	if err != nil {
		return "", "", "", &BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
	}
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, sourceFile); err != nil {
		return "", "", "", &BulkResult{ID: id, Status: bulkFailureStatus(req.Preview), Reason: err.Error()}
	}

	filename := filepath.Base(sourceFile)
	destPath = filepath.Join(req.DestinationDir, filename)
	if err := mutationguard.ValidateContentMutationRelPath(rt.VaultCfg, destPath); err != nil {
		return "", "", "", &BulkResult{ID: id, Status: bulkFailureStatus(req.Preview), Reason: err.Error()}
	}
	fullDestPath = filepath.Join(rt.VaultPath, destPath)
	if _, err := os.Stat(fullDestPath); err == nil {
		return "", "", "", &BulkResult{
			ID:     id,
			Status: "skipped",
			Reason: fmt.Sprintf("destination already exists: %s", destPath),
		}
	}
	return sourceFile, destPath, fullDestPath, nil
}

func moveBulkSectionIDs(ids []string) []string {
	var sectionIDs []string
	for _, id := range ids {
		if _, _, isSection := paths.ParseSectionID(strings.TrimSpace(id)); isSection {
			sectionIDs = append(sectionIDs, id)
		}
	}
	return sectionIDs
}
