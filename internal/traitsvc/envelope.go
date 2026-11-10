package traitsvc

import "github.com/aidanlsb/raven/internal/check"

// UpdatePreviewResult is the success payload for an update preview.
type UpdatePreviewResult struct {
	Preview bool              `json:"preview"`
	Action  string            `json:"action"`
	Items   []BulkPreviewItem `json:"items"`
	Skipped []BulkResult      `json:"skipped"`
	Total   int               `json:"total"`
}

// UpdateAppliedResult is the success payload for applied trait updates.
type UpdateAppliedResult struct {
	Action   string       `json:"action"`
	Items    []BulkResult `json:"items"`
	Total    int          `json:"total"`
	Modified int          `json:"modified"`
	Skipped  int          `json:"skipped"`
	Errors   int          `json:"errors"`
	check.MissingReferences
}

func (p *BulkPreview) Payload() UpdatePreviewResult {
	return UpdatePreviewResult{
		Preview: true, Action: p.Action, Items: p.Items, Skipped: p.Skipped, Total: p.Total,
	}
}

func (s *BulkSummary) Payload(refs check.MissingReferences) UpdateAppliedResult {
	return UpdateAppliedResult{
		Action: s.Action, Items: s.Results, Total: s.Total, Modified: s.Modified,
		Skipped: s.Skipped, Errors: s.Errors, MissingReferences: refs,
	}
}
