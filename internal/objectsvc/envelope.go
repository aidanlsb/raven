package objectsvc

import (
	"path/filepath"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

// MoveResult is the ordinary success payload for a single-target `move`.
type MoveResult struct {
	Source           string           `json:"source"`
	Destination      string           `json:"destination"`
	Preview          bool             `json:"preview,omitempty"`
	Status           string           `json:"status,omitempty"`
	UpdatedRefs      []string         `json:"updated_refs,omitempty"`
	UpdatedRefFields []RefFieldUpdate `json:"updated_ref_fields,omitempty"`
	check.MissingReferences
}

// MoveConfirmationResult is returned when a type-directory mismatch blocks a
// move. Preview deliberately remains non-omitempty because the legacy payload
// included it even when false.
type MoveConfirmationResult struct {
	Source       string `json:"source"`
	Destination  string `json:"destination"`
	Preview      bool   `json:"preview"`
	NeedsConfirm bool   `json:"needs_confirm"`
	Reason       string `json:"reason"`
}

// DeletePreviewResult is the success payload for a single-target delete preview.
type DeletePreviewResult struct {
	Preview   bool              `json:"preview"`
	ObjectID  string            `json:"object_id"`
	Behavior  string            `json:"behavior"`
	TrashDir  string            `json:"trash_dir"`
	Backlinks []model.Reference `json:"backlinks"`
}

// DeleteResult is the success payload for an applied single-target delete.
type DeleteResult struct {
	Deleted   string `json:"deleted"`
	Behavior  string `json:"behavior"`
	TrashPath string `json:"trash_path,omitempty"`
	check.MissingReferences
}

// BulkPreviewResult holds fields shared by canonical object bulk previews.
type BulkPreviewResult struct {
	Preview  bool                  `json:"preview"`
	Action   string                `json:"action"`
	Items    []BulkPreviewItem     `json:"items"`
	Skipped  []BulkResult          `json:"skipped"`
	Total    int                   `json:"total"`
	Warnings []commandexec.Warning `json:"warnings"`
}

// AddBulkPreviewResult is the success payload for an add bulk preview.
type AddBulkPreviewResult struct {
	BulkPreviewResult
	Content string `json:"content"`
}

// SetBulkPreviewResult is the success payload for a set bulk preview.
type SetBulkPreviewResult struct {
	BulkPreviewResult
	Fields map[string]string `json:"fields"`
}

// DeleteBulkPreviewResult is the success payload for a delete bulk preview.
type DeleteBulkPreviewResult struct {
	BulkPreviewResult
	Behavior string `json:"behavior"`
}

// MoveBulkPreviewResult is the success payload for a move bulk preview.
type MoveBulkPreviewResult struct {
	BulkPreviewResult
	Destination string `json:"destination"`
}

// BulkSummaryResult holds fields shared by applied object bulk mutations.
type BulkSummaryResult struct {
	OK      bool         `json:"ok"`
	Action  string       `json:"action"`
	Items   []BulkResult `json:"items"`
	Total   int          `json:"total"`
	Skipped int          `json:"skipped"`
	Errors  int          `json:"errors"`
	check.MissingReferences
}

// AddBulkResult is the success payload for an applied bulk add.
type AddBulkResult struct {
	BulkSummaryResult
	Added   int    `json:"added"`
	Content string `json:"content"`
}

// SetBulkResult is the success payload for an applied bulk set.
type SetBulkResult struct {
	BulkSummaryResult
	Modified int               `json:"modified"`
	Fields   map[string]string `json:"fields"`
}

// DeleteBulkResult is the success payload for an applied bulk delete.
type DeleteBulkResult struct {
	BulkSummaryResult
	Deleted  int    `json:"deleted"`
	Behavior string `json:"behavior"`
}

// MoveBulkResult is the success payload for an applied bulk move.
type MoveBulkResult struct {
	BulkSummaryResult
	Moved       int    `json:"moved"`
	Destination string `json:"destination"`
}

// ReclassifyBulkPreviewResult is the success payload for a reclassify preview.
type ReclassifyBulkPreviewResult struct {
	Preview bool                        `json:"preview"`
	Action  string                      `json:"action"`
	NewType string                      `json:"new_type"`
	Items   []ReclassifyBulkPreviewItem `json:"items"`
	Skipped []ReclassifyBulkResult      `json:"skipped"`
	Total   int                         `json:"total"`
}

// ReclassifyBulkAppliedResult is the success payload for applied bulk reclassification.
type ReclassifyBulkAppliedResult struct {
	OK           bool                   `json:"ok"`
	Action       string                 `json:"action"`
	NewType      string                 `json:"new_type"`
	Items        []ReclassifyBulkResult `json:"items"`
	Total        int                    `json:"total"`
	Skipped      int                    `json:"skipped"`
	Errors       int                    `json:"errors"`
	Reclassified int                    `json:"reclassified"`
	check.MissingReferences
}

// TrashListPayload is the success payload for `trash list`.
type TrashListPayload struct {
	Items            []TrashEntry `json:"items"`
	Total            int          `json:"total"`
	TrashDir         string       `json:"trash_dir"`
	DeletionBehavior string       `json:"deletion_behavior"`
}

// RestorePayload is the success payload for `restore`.
type RestorePayload struct {
	Preview     bool   `json:"preview"`
	Reference   string `json:"reference"`
	TrashPath   string `json:"trash_path"`
	RestorePath string `json:"restore_path"`
	Kind        string `json:"kind"`
	Restored    string `json:"restored,omitempty"`
	check.MissingReferences
}

// EmptyTrashPreviewPayload is the success payload for a trash empty preview.
type EmptyTrashPreviewPayload struct {
	Preview   bool         `json:"preview"`
	Items     []TrashEntry `json:"items"`
	Total     int          `json:"total"`
	TrashDir  string       `json:"trash_dir"`
	OlderThan string       `json:"older_than,omitempty"`
}

// EmptyTrashAppliedPayload is the success payload for an applied trash empty.
type EmptyTrashAppliedPayload struct {
	Preview   bool         `json:"preview"`
	Items     []TrashEntry `json:"items"`
	Total     int          `json:"total"`
	TrashDir  string       `json:"trash_dir"`
	OlderThan string       `json:"older_than,omitempty"`
	Removed   int          `json:"removed"`
}

func bulkPreviewPayload(action string, items []BulkPreviewItem, skipped []BulkResult, total int, warnings []commandexec.Warning) BulkPreviewResult {
	return BulkPreviewResult{
		Preview:  true,
		Action:   action,
		Items:    items,
		Skipped:  skipped,
		Total:    total,
		Warnings: warnings,
	}
}

func bulkSummaryPayload(action string, items []BulkResult, total, skipped, errors int, refs check.MissingReferences) BulkSummaryResult {
	return BulkSummaryResult{
		OK:                errors == 0,
		Action:            action,
		Items:             items,
		Total:             total,
		Skipped:           skipped,
		Errors:            errors,
		MissingReferences: refs,
	}
}

func (p *AddBulkPreview) Payload(content string, warnings []commandexec.Warning) AddBulkPreviewResult {
	return AddBulkPreviewResult{
		BulkPreviewResult: bulkPreviewPayload(p.Action, p.Items, p.Skipped, p.Total, warnings),
		Content:           content,
	}
}

func (s *AddBulkSummary) Payload(content string, refs check.MissingReferences) AddBulkResult {
	return AddBulkResult{
		BulkSummaryResult: bulkSummaryPayload(s.Action, s.Results, s.Total, s.Skipped, s.Errors, refs),
		Added:             s.Added,
		Content:           content,
	}
}

func (p *SetBulkPreview) Payload(fields map[string]string, warnings []commandexec.Warning) SetBulkPreviewResult {
	return SetBulkPreviewResult{
		BulkPreviewResult: bulkPreviewPayload(p.Action, p.Items, p.Skipped, p.Total, warnings),
		Fields:            fields,
	}
}

func (s *SetBulkSummary) Payload(fields map[string]string, refs check.MissingReferences) SetBulkResult {
	return SetBulkResult{
		BulkSummaryResult: bulkSummaryPayload(s.Action, s.Results, s.Total, s.Skipped, s.Errors, refs),
		Modified:          s.Modified,
		Fields:            fields,
	}
}

func (p *DeleteBulkPreview) Payload(warnings []commandexec.Warning) DeleteBulkPreviewResult {
	return DeleteBulkPreviewResult{
		BulkPreviewResult: bulkPreviewPayload(p.Action, p.Items, p.Skipped, p.Total, warnings),
		Behavior:          p.Behavior,
	}
}

func (s *DeleteBulkSummary) Payload(refs check.MissingReferences) DeleteBulkResult {
	return DeleteBulkResult{
		BulkSummaryResult: bulkSummaryPayload(s.Action, s.Results, s.Total, s.Skipped, s.Errors, refs),
		Deleted:           s.Deleted,
		Behavior:          s.Behavior,
	}
}

func (p *MoveBulkPreview) Payload() MoveBulkPreviewResult {
	return MoveBulkPreviewResult{
		BulkPreviewResult: bulkPreviewPayload(p.Action, p.Items, p.Skipped, p.Total, nil),
		Destination:       p.Destination,
	}
}

func (s *MoveBulkSummary) Payload(refs check.MissingReferences) MoveBulkResult {
	return MoveBulkResult{
		BulkSummaryResult: bulkSummaryPayload(s.Action, s.Results, s.Total, s.Skipped, s.Errors, refs),
		Moved:             s.Moved,
		Destination:       s.Destination,
	}
}

func (p *ReclassifyBulkPreview) Payload() ReclassifyBulkPreviewResult {
	return ReclassifyBulkPreviewResult{
		Preview: true,
		Action:  p.Action,
		NewType: p.NewType,
		Items:   p.Items,
		Skipped: p.Skipped,
		Total:   p.Total,
	}
}

func (s *ReclassifyBulkSummary) Payload(refs check.MissingReferences) ReclassifyBulkAppliedResult {
	return ReclassifyBulkAppliedResult{
		OK:                s.Errors == 0,
		Action:            s.Action,
		NewType:           s.NewType,
		Items:             s.Results,
		Total:             s.Total,
		Skipped:           s.Skipped,
		Errors:            s.Errors,
		Reclassified:      s.Reclassified,
		MissingReferences: refs,
	}
}

func (r *DeleteByReferenceResult) PreviewPayload(trashDir string) DeletePreviewResult {
	return DeletePreviewResult{
		Preview:   true,
		ObjectID:  r.ObjectID,
		Behavior:  r.Behavior,
		TrashDir:  trashDir,
		Backlinks: r.Backlinks,
	}
}

func (r *DeleteByReferenceResult) AppliedPayload(vaultPath string, refs check.MissingReferences) DeleteResult {
	data := DeleteResult{
		Deleted:           r.ObjectID,
		Behavior:          r.Behavior,
		MissingReferences: refs,
	}
	if r.TrashPath != "" {
		if relDest, relErr := filepath.Rel(vaultPath, r.TrashPath); relErr == nil {
			data.TrashPath = filepath.ToSlash(relDest)
		}
	}
	return data
}

func (r *MoveByReferenceResult) Envelope(preview bool, refs check.MissingReferences) any {
	if r.NeedsConfirm && r.TypeMismatch != nil {
		return MoveConfirmationResult{
			Source:       r.SourceID,
			Destination:  r.DestinationID,
			Preview:      preview,
			NeedsConfirm: true,
			Reason:       r.Reason,
		}
	}
	data := MoveResult{
		Source:      r.SourceID,
		Destination: r.DestinationID,
	}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	if len(r.UpdatedRefs) > 0 {
		data.UpdatedRefs = r.UpdatedRefs
	}
	if len(r.UpdatedRefFields) > 0 {
		data.UpdatedRefFields = r.UpdatedRefFields
	}
	if !preview {
		data.MissingReferences = refs
	}
	return data
}

func (r *ListTrashResult) Payload() TrashListPayload {
	return TrashListPayload{
		Items:            r.Entries,
		Total:            len(r.Entries),
		TrashDir:         r.TrashDir,
		DeletionBehavior: r.DeletionBehavior,
	}
}

func (r *RestoreByReferenceResult) Payload(preview bool, refs check.MissingReferences) RestorePayload {
	data := RestorePayload{
		Preview:     preview,
		Reference:   r.Entry.Reference,
		TrashPath:   r.Entry.TrashPath,
		RestorePath: r.Entry.RestorePath,
		Kind:        r.Entry.Kind,
	}
	if !preview {
		data.Restored = r.Entry.Reference
		data.MissingReferences = refs
	}
	return data
}

func (r *EmptyTrashResult) Payload(preview bool) any {
	if preview {
		return EmptyTrashPreviewPayload{
			Preview:   true,
			Items:     r.Entries,
			Total:     len(r.Entries),
			TrashDir:  r.TrashDir,
			OlderThan: r.OlderThan,
		}
	}
	return EmptyTrashAppliedPayload{
		Preview:   false,
		Items:     r.Entries,
		Total:     len(r.Entries),
		TrashDir:  r.TrashDir,
		OlderThan: r.OlderThan,
		Removed:   len(r.Entries),
	}
}
