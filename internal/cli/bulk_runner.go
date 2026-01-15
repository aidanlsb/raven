package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/ui"
)

// buildBulkPreview builds preview items + skipped results for a bulk operation.
func buildBulkPreview(action string, ids []string, warnings []Warning, previewFn func(id string) (*BulkPreviewItem, *BulkResult)) *BulkPreview {
	var items []BulkPreviewItem
	var skipped []BulkResult

	for _, id := range ids {
		item, skip := previewFn(id)
		if skip != nil {
			skipped = append(skipped, *skip)
			continue
		}
		if item != nil {
			items = append(items, *item)
		}
	}

	return &BulkPreview{
		Action:   action,
		Items:    items,
		Skipped:  skipped,
		Total:    len(ids),
		Warnings: warnings,
	}
}

func outputBulkPreview(preview *BulkPreview, extra map[string]interface{}) error {
	if isJSONOutput() {
		data := map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    preview.Items,
			"skipped":  preview.Skipped,
			"total":    preview.Total,
			"warnings": preview.Warnings,
		}
		for k, v := range extra {
			data[k] = v
		}
		outputSuccess(data, &Meta{Count: len(preview.Items)})
		return nil
	}

	PrintBulkPreview(preview)
	return nil
}

// applyBulk executes a bulk operation and returns per-ID results.
func applyBulk(ids []string, applyFn func(id string) BulkResult) []BulkResult {
	results := make([]BulkResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, applyFn(id))
	}
	return results
}

func buildBulkSummary(action string, results []BulkResult, warnings []Warning) *BulkSummary {
	var modified, deleted, added, moved, skipped, errors int

	for _, r := range results {
		switch r.Status {
		case "modified":
			modified++
		case "deleted":
			deleted++
		case "added":
			added++
		case "moved":
			moved++
		case "skipped":
			skipped++
		case "error":
			errors++
		}
	}

	return &BulkSummary{
		Action:   action,
		Results:  results,
		Total:    len(results),
		Modified: modified,
		Deleted:  deleted,
		Added:    added,
		Moved:    moved,
		Skipped:  skipped,
		Errors:   errors,
	}
}

func outputBulkSummary(summary *BulkSummary, warnings []Warning, extra map[string]interface{}) error {
	if isJSONOutput() {
		data := map[string]interface{}{
			"ok":      summary.Errors == 0,
			"action":  summary.Action,
			"results": summary.Results,
			"total":   summary.Total,
			"skipped": summary.Skipped,
			"errors":  summary.Errors,
		}

		switch summary.Action {
		case "set":
			data["modified"] = summary.Modified
		case "delete":
			data["deleted"] = summary.Deleted
		case "add":
			data["added"] = summary.Added
		case "move":
			data["moved"] = summary.Moved
		}

		for k, v := range extra {
			data[k] = v
		}

		if len(warnings) > 0 {
			outputSuccessWithWarnings(data, warnings, &Meta{Count: summary.Total - summary.Skipped - summary.Errors})
		} else {
			outputSuccess(data, &Meta{Count: summary.Total - summary.Skipped - summary.Errors})
		}
		return nil
	}

	PrintBulkSummary(summary)
	for _, w := range warnings {
		fmt.Println(ui.Warning(w.Message))
	}
	return nil
}
