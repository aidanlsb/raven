package cli

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/querysvc"
	"github.com/aidanlsb/raven/internal/traitsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

func renderCanonicalBulkResult(result commandexec.Result) error {
	if isJSONOutput() {
		return outputJSON(result)
	}

	switch data := result.Data.(type) {
	case objectsvc.AddBulkPreviewResult:
		printObjectBulkPreview(data.BulkPreviewResult)
	case objectsvc.SetBulkPreviewResult:
		printObjectBulkPreview(data.BulkPreviewResult)
	case objectsvc.DeleteBulkPreviewResult:
		printObjectBulkPreview(data.BulkPreviewResult)
	case objectsvc.MoveBulkPreviewResult:
		printObjectBulkPreview(data.BulkPreviewResult)
	case objectsvc.ReclassifyBulkPreviewResult:
		PrintBulkPreview(&BulkPreview{
			Action:  data.Action,
			Items:   reclassifyPreviewItems(data.Items),
			Skipped: reclassifyBulkResults(data.Skipped),
			Total:   data.Total,
		})
	case querysvc.QueryApplyEmptyResult:
		if data.Preview {
			PrintBulkPreview(&BulkPreview{Action: data.Action, Total: data.Total})
		} else {
			PrintBulkSummary(&BulkSummary{Action: data.Action, Total: data.Total})
		}
	case traitsvc.UpdatePreviewResult:
		printTraitBulkPreview(&traitsvc.BulkPreview{
			Action:  data.Action,
			Items:   data.Items,
			Skipped: data.Skipped,
			Total:   data.Total,
		})
	case objectsvc.AddBulkResult:
		printObjectBulkSummary(data.BulkSummaryResult, BulkSummary{Added: data.Added})
	case objectsvc.SetBulkResult:
		printObjectBulkSummary(data.BulkSummaryResult, BulkSummary{Modified: data.Modified})
	case objectsvc.DeleteBulkResult:
		printObjectBulkSummary(data.BulkSummaryResult, BulkSummary{Deleted: data.Deleted})
	case objectsvc.MoveBulkResult:
		printObjectBulkSummary(data.BulkSummaryResult, BulkSummary{Moved: data.Moved})
	case objectsvc.ReclassifyBulkAppliedResult:
		PrintBulkSummary(&BulkSummary{
			Action:       data.Action,
			Total:        data.Total,
			Reclassified: data.Reclassified,
			Skipped:      data.Skipped,
			Errors:       data.Errors,
		})
	case traitsvc.UpdateAppliedResult:
		printTraitBulkSummary(&traitsvc.BulkSummary{
			Action:   data.Action,
			Results:  data.Items,
			Total:    data.Total,
			Modified: data.Modified,
			Skipped:  data.Skipped,
			Errors:   data.Errors,
		})
	default:
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	for _, warning := range result.Warnings {
		fmt.Println(ui.Warning(warning.Message))
	}
	return nil
}

func printObjectBulkPreview(data objectsvc.BulkPreviewResult) {
	items := make([]BulkPreviewItem, len(data.Items))
	for i, item := range data.Items {
		items[i] = BulkPreviewItem{
			ID:      item.ID,
			Changes: item.Changes,
			Action:  item.Action,
			Details: item.Details,
		}
	}
	PrintBulkPreview(&BulkPreview{
		Action:   data.Action,
		Items:    items,
		Skipped:  data.Skipped,
		Total:    data.Total,
		Warnings: data.Warnings,
	})
}

func printObjectBulkSummary(data objectsvc.BulkSummaryResult, counts BulkSummary) {
	counts.Action = data.Action
	counts.Results = data.Items
	counts.Total = data.Total
	counts.Skipped = data.Skipped
	counts.Errors = data.Errors
	PrintBulkSummary(&counts)
}

func reclassifyPreviewItems(items []objectsvc.ReclassifyBulkPreviewItem) []BulkPreviewItem {
	out := make([]BulkPreviewItem, len(items))
	for i, item := range items {
		out[i] = BulkPreviewItem{
			ID:            item.ID,
			Action:        item.Action,
			OldType:       item.OldType,
			NewType:       item.NewType,
			Moved:         item.Moved,
			OldPath:       item.OldPath,
			NewPath:       item.NewPath,
			UpdatedRefs:   item.UpdatedRefs,
			AddedFields:   item.AddedFields,
			DroppedFields: item.DroppedFields,
			NeedsConfirm:  item.NeedsConfirm,
		}
	}
	return out
}

func reclassifyBulkResults(items []objectsvc.ReclassifyBulkResult) []BulkResult {
	out := make([]BulkResult, len(items))
	for i, item := range items {
		out[i] = BulkResult{
			ID:     item.ID,
			Status: item.Status,
			Reason: item.Reason,
		}
	}
	return out
}
