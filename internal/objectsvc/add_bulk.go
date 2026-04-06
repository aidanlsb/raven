package objectsvc

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
)

type AddBulkRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	ObjectIDs    []string
	Line         string
	HeadingSpec  string
	ParseOptions *parser.ParseOptions
}

type AddBulkPreviewItem struct {
	ID      string
	Action  string
	Details string
}

type AddBulkResult struct {
	ID     string
	Status string
	Reason string
}

type AddBulkPreview struct {
	Action  string
	Items   []AddBulkPreviewItem
	Skipped []AddBulkResult
	Total   int
}

type AddBulkSummary struct {
	Action  string
	Results []AddBulkResult
	Total   int
	Skipped int
	Errors  int
	Added   int
}

func PreviewAddBulk(req AddBulkRequest) (*AddBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	items := make([]AddBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]AddBulkResult, 0)

	for _, id := range req.ObjectIDs {
		fileID := id
		targetObjectID := ""
		if baseID, _, isEmbedded := paths.ParseEmbeddedID(id); isEmbedded {
			fileID = baseID
			targetObjectID = id
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, fileID, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}
		if err := ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
			skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
			continue
		}
		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: "file not found"})
			continue
		}

		if strings.Contains(id, "#") {
			content, err := os.ReadFile(filePath)
			if err != nil {
				skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)})
				continue
			}
			doc, err := parser.ParseDocumentWithOptions(string(content), filePath, req.VaultPath, req.ParseOptions)
			if err != nil {
				skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("parse error: %v", err)})
				continue
			}
			found := false
			for _, obj := range doc.Objects {
				if obj != nil && obj.ID == id {
					found = true
					break
				}
			}
			if !found {
				skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: "embedded object not found"})
				continue
			}
		}

		if req.HeadingSpec != "" {
			if targetObjectID != "" {
				skipped = append(skipped, AddBulkResult{
					ID:     id,
					Status: "skipped",
					Reason: "cannot combine --heading with embedded IDs from stdin",
				})
				continue
			}
			resolvedTarget, err := ResolveAddHeadingTarget(req.VaultPath, filePath, fileID, req.HeadingSpec, req.ParseOptions)
			if err != nil {
				skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: err.Error()})
				continue
			}
			targetObjectID = resolvedTarget
		}

		details := fmt.Sprintf("append: %s", req.Line)
		if targetObjectID != "" {
			details = fmt.Sprintf("append within %s: %s", targetObjectID, req.Line)
		}
		items = append(items, AddBulkPreviewItem{
			ID:      id,
			Action:  "add",
			Details: details,
		})
	}

	return &AddBulkPreview{
		Action:  "add",
		Items:   items,
		Skipped: skipped,
		Total:   len(req.ObjectIDs),
	}, nil
}

func ApplyAddBulk(req AddBulkRequest, onAdded func(filePath string)) (*AddBulkSummary, error) {
	if req.VaultConfig == nil {
		return nil, newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}

	results := make([]AddBulkResult, 0, len(req.ObjectIDs))
	addedCount := 0
	skippedCount := 0
	errorCount := 0
	captureCfg := req.VaultConfig.GetCaptureConfig()

	for _, id := range req.ObjectIDs {
		result := AddBulkResult{ID: id}
		fileID := id
		targetObjectID := ""
		if baseID, _, isEmbedded := paths.ParseEmbeddedID(id); isEmbedded {
			fileID = baseID
			targetObjectID = id
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, fileID, req.VaultConfig)
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

		if req.HeadingSpec != "" {
			if targetObjectID != "" {
				result.Status = "error"
				result.Reason = "cannot combine --heading with embedded IDs from stdin"
				errorCount++
				results = append(results, result)
				continue
			}
			resolvedTarget, err := ResolveAddHeadingTarget(req.VaultPath, filePath, fileID, req.HeadingSpec, req.ParseOptions)
			if err != nil {
				result.Status = "error"
				result.Reason = err.Error()
				errorCount++
				results = append(results, result)
				continue
			}
			targetObjectID = resolvedTarget
		}

		if _, err := AppendToFile(req.VaultPath, filePath, req.Line, captureCfg, req.VaultConfig, false, targetObjectID, req.ParseOptions); err != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("append failed: %v", err)
			errorCount++
			results = append(results, result)
			continue
		}

		if onAdded != nil {
			onAdded(filePath)
		}

		result.Status = "added"
		addedCount++
		results = append(results, result)
	}

	return &AddBulkSummary{
		Action:  "add",
		Results: results,
		Total:   len(results),
		Skipped: skippedCount,
		Errors:  errorCount,
		Added:   addedCount,
	}, nil
}
