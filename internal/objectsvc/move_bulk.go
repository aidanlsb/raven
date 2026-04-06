package objectsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

type MoveBulkRequest struct {
	VaultPath      string
	VaultConfig    *config.VaultConfig
	Schema         *schema.Schema
	ObjectIDs      []string
	DestinationDir string
	UpdateRefs     bool
	ParseOptions   *parser.ParseOptions
}

type MoveBulkPreviewItem struct {
	ID      string
	Action  string
	Details string
}

type MoveBulkResult struct {
	ID      string
	Status  string
	Reason  string
	Details string
}

type MoveBulkPreview struct {
	Action      string
	Items       []MoveBulkPreviewItem
	Skipped     []MoveBulkResult
	Total       int
	Destination string
}

type MoveBulkSummary struct {
	Action          string
	Results         []MoveBulkResult
	Total           int
	Skipped         int
	Errors          int
	Moved           int
	Destination     string
	WarningMessages []string
}

func PreviewMoveBulk(req MoveBulkRequest) (*MoveBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if !strings.HasSuffix(req.DestinationDir, "/") {
		return nil, newError(ErrorInvalidInput, "destination must be a directory (end with /)", "Example: rvn move --stdin archive/projects/", nil, nil)
	}

	items := make([]MoveBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]MoveBulkResult, 0)
	for _, id := range req.ObjectIDs {
		sourceFile, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, MoveBulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
			skipped = append(skipped, MoveBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(req.DestinationDir, filename)
		if err := ValidateContentMutationRelPath(req.VaultConfig, destPath); err != nil {
			skipped = append(skipped, MoveBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}
		fullDestPath := filepath.Join(req.VaultPath, destPath)
		if _, err := os.Stat(fullDestPath); err == nil {
			skipped = append(skipped, MoveBulkResult{
				ID:     id,
				Status: "skipped",
				Reason: fmt.Sprintf("destination already exists: %s", destPath),
			})
			continue
		}

		items = append(items, MoveBulkPreviewItem{
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
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if !strings.HasSuffix(req.DestinationDir, "/") {
		return nil, newError(ErrorInvalidInput, "destination must be a directory (end with /)", "Example: rvn move --stdin archive/projects/", nil, nil)
	}

	results := make([]MoveBulkResult, 0, len(req.ObjectIDs))
	movedCount := 0
	skippedCount := 0
	errorCount := 0
	warnings := make([]string, 0)

	for _, id := range req.ObjectIDs {
		result := MoveBulkResult{ID: id}

		sourceFile, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, id, req.VaultConfig)
		if err != nil {
			result.Status = "skipped"
			result.Reason = "object not found"
			skippedCount++
			results = append(results, result)
			continue
		}
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, sourceFile); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}

		filename := filepath.Base(sourceFile)
		destPath := filepath.Join(req.DestinationDir, filename)
		if err := ValidateContentMutationRelPath(req.VaultConfig, destPath); err != nil {
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
			FailOnIndexError:  true,
			VaultConfig:       req.VaultConfig,
			Schema:            req.Schema,
			ParseOptions:      req.ParseOptions,
		})
		if err != nil {
			result.Status = "error"
			var svcErr *Error
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
	}, nil
}
