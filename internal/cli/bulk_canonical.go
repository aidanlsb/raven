package cli

import (
	"encoding/json"
	"fmt"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/traitsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

func renderCanonicalBulkResult(result commandexec.Result) error {
	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data, ok := result.Data.(map[string]interface{})
	if !ok {
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if preview, _ := data["preview"].(bool); preview {
		action, _ := data["action"].(string)
		if action == "update-trait" {
			var preview traitsvc.BulkPreview
			if err := decodeResultData(data, &preview); err != nil {
				return handleError(ErrInternal, err, "")
			}
			printTraitBulkPreview(&preview)
			return nil
		}

		preview := &BulkPreview{
			Action:   action,
			Items:    decodeBulkPreviewItems(data["items"]),
			Skipped:  decodeBulkResults(data["skipped"]),
			Total:    intFromAny(data["total"]),
			Warnings: decodeWarnings(data["warnings"]),
		}
		PrintBulkPreview(preview)
		return nil
	}

	action, _ := data["action"].(string)
	if action == "update-trait" {
		var summary traitsvc.BulkSummary
		if err := decodeResultData(data, &summary); err != nil {
			return handleError(ErrInternal, err, "")
		}
		printTraitBulkSummary(&summary)
		return nil
	}

	summary := &BulkSummary{
		Action:   action,
		Results:  decodeBulkResults(data["results"]),
		Total:    intFromAny(data["total"]),
		Modified: intFromAny(data["modified"]),
		Deleted:  intFromAny(data["deleted"]),
		Added:    intFromAny(data["added"]),
		Moved:    intFromAny(data["moved"]),
		Skipped:  intFromAny(data["skipped"]),
		Errors:   intFromAny(data["errors"]),
	}
	PrintBulkSummary(summary)
	for _, warning := range result.Warnings {
		fmt.Println(ui.Warning(warning.Message))
	}
	return nil
}

func decodeBulkPreviewItems(raw interface{}) []BulkPreviewItem {
	var items []BulkPreviewItem
	_ = decodeResultData(raw, &items)
	return items
}

func decodeBulkResults(raw interface{}) []BulkResult {
	var items []BulkResult
	_ = decodeResultData(raw, &items)
	return items
}

func decodeWarnings(raw interface{}) []Warning {
	var warnings []Warning
	_ = decodeResultData(raw, &warnings)
	return warnings
}

func decodeResultData(raw interface{}, out interface{}) error {
	if raw == nil {
		return nil
	}
	b, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, out)
}
