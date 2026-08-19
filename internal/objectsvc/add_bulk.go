package objectsvc

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type AddBulkRequest struct {
	VaultPath    string
	VaultConfig  *config.VaultConfig
	ObjectIDs    []string
	Line         string
	ParseOptions *parser.ParseOptions
	Runtime      *vaultruntime.Runtime
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
	Action    string
	Results   []AddBulkResult
	Total     int
	Skipped   int
	Errors    int
	Added     int
	ChangeSet mutation.ChangeSet
}

func PreviewAddBulk(req AddBulkRequest) (*AddBulkPreview, error) {
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}

	items := make([]AddBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]AddBulkResult, 0)

	for _, id := range req.ObjectIDs {
		fileID := id
		targetObjectID := ""
		if baseID, _, isSection := paths.ParseSectionID(id); isSection {
			fileID = baseID
			targetObjectID = id
		}

		filePath, err := vault.ResolveObjectToFileWithConfig(req.VaultPath, fileID, req.VaultConfig)
		if err != nil {
			skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: "object not found"})
			continue
		}
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
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
			for _, section := range doc.Sections {
				if section != nil && section.ID == id {
					found = true
					break
				}
			}
			if !found {
				skipped = append(skipped, AddBulkResult{ID: id, Status: "skipped", Reason: "section not found"})
				continue
			}
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

func ApplyAddBulk(req AddBulkRequest) (*AddBulkSummary, error) {
	if req.VaultConfig == nil {
		return nil, svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}

	results := make([]AddBulkResult, 0, len(req.ObjectIDs))
	addedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()
	captureCfg := req.VaultConfig.GetCaptureConfig()
	rt, owned := vaultruntime.FromRequest(req.Runtime, req.VaultPath, req.VaultConfig, nil, req.ParseOptions)
	if owned {
		defer rt.Close()
	}

	for _, id := range req.ObjectIDs {
		result := AddBulkResult{ID: id}
		fileID := id
		targetObjectID := ""
		if baseID, _, isSection := paths.ParseSectionID(id); isSection {
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
		if err := mutationguard.ValidateContentMutationFilePath(req.VaultPath, req.VaultConfig, filePath); err != nil {
			result.Status = "error"
			result.Reason = err.Error()
			errorCount++
			results = append(results, result)
			continue
		}

		appendResult, appendErr := Append(rt, filePath, req.Line, captureCfg, false, targetObjectID)
		if appendErr != nil {
			result.Status = "error"
			result.Reason = fmt.Sprintf("append failed: %v", appendErr)
			errorCount++
			results = append(results, result)
			continue
		}

		changes.Merge(appendResult.ChangeSet)

		result.Status = "added"
		addedCount++
		results = append(results, result)
	}

	return &AddBulkSummary{
		Action:    "add",
		Results:   results,
		Total:     len(results),
		Skipped:   skippedCount,
		Errors:    errorCount,
		Added:     addedCount,
		ChangeSet: changes,
	}, nil
}
