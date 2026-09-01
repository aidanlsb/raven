package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type MoveBulkRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	ObjectIDs      []string
	DestinationDir string
	UpdateRefs     bool
	ParseOptions   *parser.ParseOptions
	Runtime        *vaultruntime.Runtime
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

func PreviewMoveBulk(req MoveBulkRequest) (*MoveBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	if !strings.HasSuffix(req.DestinationDir, "/") {
		return nil, svcerr.New(codes.ErrInvalidInput, "destination must be a directory (end with /)").WithSuggestion("Example: rvn move --stdin archive/projects/")
	}
	if sectionIDs := moveBulkSectionIDs(req.ObjectIDs); len(sectionIDs) > 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "rvn move does not accept section sources").WithSuggestion(`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`).WithDetails(map[string]interface{}{"section_ids": sectionIDs})
	}

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	for _, id := range req.ObjectIDs {
		sourceFile, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(req.DestinationDir, filename)
		if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, destPath); err != nil {
			skipped = append(skipped, BulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}
		fullDestPath := filepath.Join(req.VaultPath, destPath)
		if _, err := os.Stat(fullDestPath); err == nil {
			skipped = append(skipped, BulkResult{
				ID:     id,
				Status: "skipped",
				Reason: fmt.Sprintf("destination already exists: %s", destPath),
			})
			continue
		}

		items = append(items, BulkPreviewItem{
			ID:      id,
			Action:  "move",
			Details: fmt.Sprintf("→ %s", destPath),
		})
	}

	return &MoveBulkPreview{
		Action:      "move",
		Items:       items,
		Skipped:     skipped,
		Total:       len(req.ObjectIDs),
		Destination: req.DestinationDir,
	}, nil
}

func ApplyMoveBulk(req MoveBulkRequest) (*MoveBulkSummary, error) {
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	if !strings.HasSuffix(req.DestinationDir, "/") {
		return nil, svcerr.New(codes.ErrInvalidInput, "destination must be a directory (end with /)").WithSuggestion("Example: rvn move --stdin archive/projects/")
	}
	if sectionIDs := moveBulkSectionIDs(req.ObjectIDs); len(sectionIDs) > 0 {
		return nil, svcerr.New(codes.ErrInvalidInput, "rvn move does not accept section sources").WithSuggestion(`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`).WithDetails(map[string]interface{}{"section_ids": sectionIDs})
	}

	results := make([]BulkResult, 0, len(req.ObjectIDs))
	movedCount := 0
	skippedCount := 0
	errorCount := 0
	warnings := make([]string, 0)
	changes := mutation.NewChangeSet()

	for _, id := range req.ObjectIDs {
		result := BulkResult{ID: id}

		sourceFile, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(req.DestinationDir, filename)
		if err := mutationguard.ValidateContentMutationRelPath(req.VaultConfig, destPath); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}
		fullDestPath := filepath.Join(req.VaultPath, destPath)
		if _, err := os.Stat(fullDestPath); err == nil {
			result.Status = "skipped"
			result.Reason = fmt.Sprintf("destination already exists: %s", destPath)
			skippedCount++
			results = append(results, result)
			continue
		}

		relSource, _ := filepath.Rel(req.VaultPath, sourceFile)
		sourceID := req.VaultConfig.FilePathToObjectID(relSource)
		destID := req.VaultConfig.FilePathToObjectID(destPath)

		serviceResult, err := MoveFile(MoveFileRequest{
			VaultPath:         req.VaultPath,
			SourceFile:        sourceFile,
			DestinationFile:   fullDestPath,
			SourceObjectID:    sourceID,
			DestinationObject: destID,
			UpdateRefs:        req.UpdateRefs,
			PriorMoves:        append([]mutation.Move(nil), changes.Moved...),
			VaultConfig:       req.VaultConfig,
			Schema:            req.Schema,
			ParseOptions:      req.ParseOptions,
			Runtime:           req.Runtime,
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

	return &MoveBulkSummary{
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

func moveBulkSectionIDs(ids []string) []string {
	var sectionIDs []string
	for _, id := range ids {
		if _, _, isSection := paths.ParseSectionID(strings.TrimSpace(id)); isSection {
			sectionIDs = append(sectionIDs, id)
		}
	}
	return sectionIDs
}
