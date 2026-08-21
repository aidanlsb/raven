package commandpayload

import (
	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checkfixsvc"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/fieldvalue"
	"github.com/aidanlsb/raven/internal/importsvc"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/traitsvc"
)

// MissingReferences is embedded in mutation payloads that can create dangling
// references. Both fields are absent when the mutation did not introduce any
// missing targets, preserving the historical conditional map fields.
type MissingReferences struct {
	MissingRefs     int                 `json:"missing_refs,omitempty"`
	MissingRefItems []*check.MissingRef `json:"missing_ref_items,omitempty"`
}

// MissingReferenceItems lets CLI renderers consume the shared missing-reference
// block without decoding a command-specific payload back through JSON.
func (m MissingReferences) MissingReferenceItems() []*check.MissingRef {
	return m.MissingRefItems
}

// MissingReferencePayload is implemented by mutation payloads that embed
// MissingReferences.
type MissingReferencePayload interface {
	MissingReferenceItems() []*check.MissingRef
}

// ObjectMutation identifies a file-backed object created or upserted by a
// mutation command.
type ObjectMutation struct {
	ID    string `json:"id"`
	File  string `json:"file"`
	Title string `json:"title"`
	Type  string `json:"type"`
}

// NewResult is the success payload for `new`.
type NewResult struct {
	ObjectMutation
	MissingReferences
}

// UpsertResult is the success payload for `upsert`.
type UpsertResult struct {
	Status string `json:"status"`
	ObjectMutation
	MissingReferences
}

// AddResult is the success payload for a single-target `add`.
type AddResult struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Content string `json:"content"`
	MissingReferences
}

// SetResult is the success payload for a single-target `set`.
type SetResult struct {
	File           string                           `json:"file"`
	ObjectID       string                           `json:"object_id"`
	Type           string                           `json:"type"`
	UpdatedFields  map[string]string                `json:"updated_fields"`
	PreviousFields map[string]fieldvalue.FieldValue `json:"previous_fields,omitempty"`
	Preview        bool                             `json:"preview,omitempty"`
	MissingReferences
}

// UnsetResult is the success payload for `unset`.
type UnsetResult struct {
	File           string                           `json:"file"`
	ObjectID       string                           `json:"object_id"`
	Type           string                           `json:"type"`
	RemovedFields  map[string]string                `json:"removed_fields"`
	MissingFields  []string                         `json:"missing_fields"`
	Modified       bool                             `json:"modified"`
	PreviousFields map[string]fieldvalue.FieldValue `json:"previous_fields"`
	MissingReferences
}

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
	MissingReferences
}

// EditBatchResult is the success payload for applied batch edits.
type EditBatchResult struct {
	Status string            `json:"status"`
	Path   string            `json:"path"`
	Count  int               `json:"count"`
	Edits  []EditAppliedItem `json:"edits"`
	MissingReferences
}

// MoveResult is the ordinary success payload for a single-target `move`.
type MoveResult struct {
	Source           string               `json:"source"`
	Destination      string               `json:"destination"`
	Preview          bool                 `json:"preview,omitempty"`
	Status           string               `json:"status,omitempty"`
	UpdatedRefs      []string             `json:"updated_refs,omitempty"`
	UpdatedRefFields []MoveRefFieldUpdate `json:"updated_ref_fields,omitempty"`
	MissingReferences
}

// MoveRefFieldUpdate describes one frontmatter ref field rewritten by `move`.
type MoveRefFieldUpdate struct {
	SourceID string `json:"source_id"`
	File     string `json:"file"`
	Field    string `json:"field"`
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
	MissingReferences
}

// ReclassifyResult is the success payload for a single-target `reclassify`.
// Fields intentionally do not use omitempty because the legacy map always
// emitted the complete result shape.
type ReclassifyResult struct {
	ObjectID      string   `json:"object_id"`
	OldType       string   `json:"old_type"`
	NewType       string   `json:"new_type"`
	File          string   `json:"file"`
	Moved         bool     `json:"moved"`
	OldPath       string   `json:"old_path"`
	NewPath       string   `json:"new_path"`
	UpdatedRefs   []string `json:"updated_refs"`
	AddedFields   []string `json:"added_fields"`
	DroppedFields []string `json:"dropped_fields"`
	NeedsConfirm  bool     `json:"needs_confirm"`
	Reason        string   `json:"reason"`
	MissingReferences
}

// BulkResult is one canonical item in an add/set/delete/move bulk result.
type BulkResult struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Reason  string `json:"reason,omitempty"`
	Details string `json:"details,omitempty"`
}

// BulkPreviewItem is one canonical add/set/delete/move preview item.
type BulkPreviewItem struct {
	ID      string            `json:"id"`
	Changes map[string]string `json:"changes,omitempty"`
	Action  string            `json:"action"`
	Details string            `json:"details,omitempty"`
}

// BulkPreviewResult holds fields shared by canonical object bulk previews.
type BulkPreviewResult struct {
	Preview bool              `json:"preview"`
	Action  string            `json:"action"`
	Items   []BulkPreviewItem `json:"items"`
	Skipped []BulkResult      `json:"skipped"`
	Total   int               `json:"total"`
}

// AddBulkPreviewResult is the success payload for an add bulk preview.
type AddBulkPreviewResult struct {
	BulkPreviewResult
	Warnings []commandexec.Warning `json:"warnings"`
	Content  string                `json:"content"`
}

// SetBulkPreviewResult is the success payload for a set bulk preview.
type SetBulkPreviewResult struct {
	BulkPreviewResult
	Warnings []commandexec.Warning `json:"warnings"`
	Fields   map[string]string     `json:"fields"`
}

// DeleteBulkPreviewResult is the success payload for a delete bulk preview.
type DeleteBulkPreviewResult struct {
	BulkPreviewResult
	Warnings []commandexec.Warning `json:"warnings"`
	Behavior string                `json:"behavior"`
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
	MissingReferences
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

// QueryApplyEmptyResult is the no-match payload for `query --apply`. It keeps
// the legacy minimal bulk shape because no nested mutation command runs.
type QueryApplyEmptyResult struct {
	Preview bool          `json:"preview"`
	Action  string        `json:"action"`
	Items   []interface{} `json:"items"`
	Total   int           `json:"total"`
}

// TraitUpdatePreviewResult is the success payload for an update preview.
type TraitUpdatePreviewResult struct {
	Preview bool                       `json:"preview"`
	Action  string                     `json:"action"`
	Items   []traitsvc.BulkPreviewItem `json:"items"`
	Skipped []traitsvc.BulkResult      `json:"skipped"`
	Total   int                        `json:"total"`
}

// TraitUpdateResult is the success payload for applied trait updates.
type TraitUpdateResult struct {
	Action   string                `json:"action"`
	Items    []traitsvc.BulkResult `json:"items"`
	Total    int                   `json:"total"`
	Modified int                   `json:"modified"`
	Skipped  int                   `json:"skipped"`
	Errors   int                   `json:"errors"`
	MissingReferences
}

// ReclassifyBulkPreviewResult is the success payload for a reclassify preview.
type ReclassifyBulkPreviewResult struct {
	Preview bool                                  `json:"preview"`
	Action  string                                `json:"action"`
	NewType string                                `json:"new_type"`
	Items   []objectsvc.ReclassifyBulkPreviewItem `json:"items"`
	Skipped []objectsvc.ReclassifyBulkResult      `json:"skipped"`
	Total   int                                   `json:"total"`
}

// ReclassifyBulkResult is the success payload for applied bulk reclassification.
type ReclassifyBulkResult struct {
	OK           bool                             `json:"ok"`
	Action       string                           `json:"action"`
	NewType      string                           `json:"new_type"`
	Items        []objectsvc.ReclassifyBulkResult `json:"items"`
	Total        int                              `json:"total"`
	Skipped      int                              `json:"skipped"`
	Errors       int                              `json:"errors"`
	Reclassified int                              `json:"reclassified"`
	MissingReferences
}

// ImportResult is the success payload for `import` in preview or apply mode.
type ImportResult struct {
	Total   int                    `json:"total"`
	Created int                    `json:"created"`
	Updated int                    `json:"updated"`
	Skipped int                    `json:"skipped"`
	Errors  int                    `json:"errors"`
	Items   []importsvc.ResultItem `json:"items"`
	MissingReferences
}

// CheckScope is the stable scope object shared by check mutation payloads.
type CheckScope struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// CheckFixPreviewResult is the success payload for `check fix` before confirm.
type CheckFixPreviewResult struct {
	Preview       bool                    `json:"preview"`
	FixableIssues int                     `json:"fixable_issues"`
	Files         []checkfixsvc.FileFixes `json:"files"`
	Scope         CheckScope              `json:"scope"`
	FileCount     int                     `json:"file_count"`
	ErrorCount    int                     `json:"error_count"`
	WarningCount  int                     `json:"warning_count"`
}

// CheckFixResult is the success payload for an applied `check fix`.
type CheckFixResult struct {
	Preview       bool                     `json:"preview"`
	OK            bool                     `json:"ok"`
	FixableIssues int                      `json:"fixable_issues"`
	FixedIssues   int                      `json:"fixed_issues"`
	FixedFiles    int                      `json:"fixed_files"`
	SkippedIssues int                      `json:"skipped_issues"`
	SkippedItems  []checkfixsvc.SkippedFix `json:"skipped_items"`
	Scope         CheckScope               `json:"scope"`
	FileCount     int                      `json:"file_count"`
	ErrorCount    int                      `json:"error_count"`
	WarningCount  int                      `json:"warning_count"`
	MissingReferences
}

// CheckCreateMissingResult is the success payload for `check create-missing`.
// Pointer fields distinguish applied-only keys from preview keys without
// dropping legitimate zero and false values.
type CheckCreateMissingResult struct {
	Preview             bool                                `json:"preview"`
	MissingRefs         int                                 `json:"missing_refs"`
	UndefinedTraits     int                                 `json:"undefined_traits"`
	RequiresConfirm     bool                                `json:"requires_confirm"`
	NonInteractiveOnly  bool                                `json:"non_interactive_only"`
	Scope               CheckScope                          `json:"scope"`
	MissingRefItems     []*check.MissingRef                 `json:"missing_ref_items"`
	UndefinedTraitItems []*check.UndefinedTrait             `json:"undefined_trait_items"`
	FileCount           int                                 `json:"file_count"`
	ErrorCount          int                                 `json:"error_count"`
	WarningCount        int                                 `json:"warning_count"`
	OK                  *bool                               `json:"ok,omitempty"`
	CreatedPages        *int                                `json:"created_pages,omitempty"`
	FailedPages         *int                                `json:"failed_pages,omitempty"`
	FailedPageItems     *[]checkfixsvc.CreateMissingFailure `json:"failed_page_items,omitempty"`
	UndefinedTraitsNote string                              `json:"undefined_traits_note,omitempty"`
}

// CheckResultScope returns the scope of any typed check mutation payload.
func CheckResultScope(data any) (checksvc.Scope, bool) {
	switch payload := data.(type) {
	case CheckFixPreviewResult:
		return checksvc.Scope{Type: payload.Scope.Type, Value: payload.Scope.Value}, true
	case CheckFixResult:
		return checksvc.Scope{Type: payload.Scope.Type, Value: payload.Scope.Value}, true
	case CheckCreateMissingResult:
		return checksvc.Scope{Type: payload.Scope.Type, Value: payload.Scope.Value}, true
	default:
		return checksvc.Scope{}, false
	}
}

// SectionLifecycleResult is the success payload for section create/move.
type SectionLifecycleResult struct {
	Section   string `json:"section"`
	File      string `json:"file"`
	Placement string `json:"placement"`
	Anchor    string `json:"anchor,omitempty"`
	Preview   bool   `json:"preview,omitempty"`
	Status    string `json:"status,omitempty"`
	Level     int    `json:"level,omitempty"`
}

// SectionRenameResult is the success payload for section rename.
type SectionRenameResult struct {
	Source      string   `json:"source"`
	Destination string   `json:"destination"`
	Preview     bool     `json:"preview,omitempty"`
	Status      string   `json:"status,omitempty"`
	UpdatedRefs []string `json:"updated_refs,omitempty"`
}

// CancelledResult is emitted by interactive CLI flows that the user cancels.
type CancelledResult struct {
	Cancelled bool `json:"cancelled"`
}
