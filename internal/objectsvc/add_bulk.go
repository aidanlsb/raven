package objectsvc

import (
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/mutationguard"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type AddBulkRequest struct {
	ObjectIDs []string
	Line      string
	Preview   bool
}

type AddBulkPreview struct {
	Action  string
	Items   []BulkPreviewItem
	Skipped []BulkResult
	Total   int
}

type AddBulkSummary struct {
	Action    string
	Results   []BulkResult
	Total     int
	Skipped   int
	Errors    int
	Added     int
	ChangeSet mutation.ChangeSet
}

func AddBulk(rt *vaultruntime.Runtime, req AddBulkRequest) (*AddBulkPreview, *AddBulkSummary, error) {
	if err := ValidateAddContent(req.Line); err != nil {
		return nil, nil, err
	}
	if err := requireVaultConfig(rt); err != nil {
		return nil, nil, err
	}

	items := make([]BulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]BulkResult, 0)
	results := make([]BulkResult, 0, len(req.ObjectIDs))
	addedCount := 0
	skippedCount := 0
	errorCount := 0
	changes := mutation.NewChangeSet()
	captureCfg := rt.VaultCfg.GetCaptureConfig()

	for _, id := range req.ObjectIDs {
		filePath, targetObjectID, fail := resolveAddBulkItem(rt, id, req.Preview)
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
			if skip := previewAddBulkItem(rt, id, filePath); skip != nil {
				skipped = append(skipped, *skip)
				continue
			}
			details := fmt.Sprintf("append: %s", req.Line)
			if targetObjectID != "" {
				details = fmt.Sprintf("append within %s: %s", targetObjectID, req.Line)
			}
			items = append(items, BulkPreviewItem{
				ID:      id,
				Action:  "add",
				Details: details,
			})
			continue
		}

		result := BulkResult{ID: id}
		appendResult, appendErr := appendLine(rt, filePath, req.Line, captureCfg, false, targetObjectID)
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

	if req.Preview {
		return &AddBulkPreview{
			Action:  "add",
			Items:   items,
			Skipped: skipped,
			Total:   len(req.ObjectIDs),
		}, nil, nil
	}
	return nil, &AddBulkSummary{
		Action:    "add",
		Results:   results,
		Total:     len(results),
		Skipped:   skippedCount,
		Errors:    errorCount,
		Added:     addedCount,
		ChangeSet: changes,
	}, nil
}

func resolveAddBulkItem(rt *vaultruntime.Runtime, id string, preview bool) (filePath, targetObjectID string, fail *BulkResult) {
	fileID := id
	if baseID, _, isSection := paths.ParseSectionID(id); isSection {
		fileID = baseID
		targetObjectID = id
	}

	filePath, err := vault.ResolveObjectToFileWithConfig(rt.VaultPath, fileID, rt.VaultCfg)
	if err != nil {
		return "", "", &BulkResult{ID: id, Status: "skipped", Reason: "object not found"}
	}
	if err := mutationguard.ValidateContentMutationFilePath(rt.VaultPath, rt.VaultCfg, filePath); err != nil {
		return "", "", &BulkResult{ID: id, Status: bulkFailureStatus(preview), Reason: err.Error()}
	}
	return filePath, targetObjectID, nil
}

func previewAddBulkItem(rt *vaultruntime.Runtime, id, filePath string) *BulkResult {
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return &BulkResult{ID: id, Status: "skipped", Reason: "file not found"}
	}
	if !strings.Contains(id, "#") {
		return nil
	}
	content, err := os.ReadFile(filePath)
	if err != nil {
		return &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("read error: %v", err)}
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, rt.VaultPath, rt.ParseOptions)
	if err != nil {
		return &BulkResult{ID: id, Status: "skipped", Reason: fmt.Sprintf("parse error: %v", err)}
	}
	for _, section := range doc.Sections {
		if section != nil && section.ID == id {
			return nil
		}
	}
	return &BulkResult{ID: id, Status: "skipped", Reason: "section not found"}
}
