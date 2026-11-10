package editsvc

import "github.com/aidanlsb/raven/internal/check"

// EditPreview is the before/after context emitted by edit previews.
type EditPreview struct {
	Before string `json:"before"`
	After  string `json:"after"`
}

// EditPreviewItem is one entry in a batch edit preview.
type EditPreviewItem struct {
	Index   int         `json:"index"`
	Line    int         `json:"line"`
	OldStr  string      `json:"old_str"`
	NewStr  string      `json:"new_str"`
	Preview EditPreview `json:"preview"`
}

// EditAppliedItem is one entry in an applied batch edit.
type EditAppliedItem struct {
	Index   int    `json:"index"`
	Line    int    `json:"line"`
	OldStr  string `json:"old_str"`
	NewStr  string `json:"new_str"`
	Context string `json:"context"`
}

// EditSinglePreviewResult is the success payload for a single edit preview.
type EditSinglePreviewResult struct {
	Status  string      `json:"status"`
	Path    string      `json:"path"`
	Line    int         `json:"line"`
	Preview EditPreview `json:"preview"`
}

// EditBatchPreviewResult is the success payload for a batch edit preview.
type EditBatchPreviewResult struct {
	Status string            `json:"status"`
	Path   string            `json:"path"`
	Count  int               `json:"count"`
	Edits  []EditPreviewItem `json:"edits"`
}

// EditSingleResult is the success payload for one applied edit.
type EditSingleResult struct {
	Status  string `json:"status"`
	Path    string `json:"path"`
	Line    int    `json:"line"`
	OldStr  string `json:"old_str"`
	NewStr  string `json:"new_str"`
	Context string `json:"context"`
	check.MissingReferences
}

// EditBatchResult is the success payload for applied batch edits.
type EditBatchResult struct {
	Status string            `json:"status"`
	Path   string            `json:"path"`
	Count  int               `json:"count"`
	Edits  []EditAppliedItem `json:"edits"`
	check.MissingReferences
}

// Envelope maps a RunResult onto the stable edit payload.
func (r *RunResult) Envelope(batch, preview bool, missing check.MissingReferences) any {
	if preview {
		if batch {
			edits := make([]EditPreviewItem, 0, len(r.Edits))
			for _, editResult := range r.Edits {
				edits = append(edits, EditPreviewItem{
					Index: editResult.Index, Line: editResult.Line,
					OldStr: editResult.OldStr, NewStr: editResult.NewStr,
					Preview: EditPreview{Before: editResult.Before, After: editResult.After},
				})
			}
			return EditBatchPreviewResult{Status: "preview", Path: r.Path, Count: len(edits), Edits: edits}
		}
		editResult := r.Edits[0]
		return EditSinglePreviewResult{
			Status: "preview", Path: r.Path, Line: editResult.Line,
			Preview: EditPreview{Before: editResult.Before, After: editResult.After},
		}
	}
	if batch {
		applied := make([]EditAppliedItem, 0, len(r.Edits))
		for _, editResult := range r.Edits {
			applied = append(applied, EditAppliedItem{
				Index: editResult.Index, Line: editResult.Line,
				OldStr: editResult.OldStr, NewStr: editResult.NewStr, Context: editResult.Context,
			})
		}
		return EditBatchResult{Status: "applied", Path: r.Path, Count: len(applied), Edits: applied, MissingReferences: missing}
	}
	editResult := r.Edits[0]
	return EditSingleResult{
		Status: "applied", Path: r.Path, Line: editResult.Line,
		OldStr: editResult.OldStr, NewStr: editResult.NewStr, Context: editResult.Context,
		MissingReferences: missing,
	}
}
