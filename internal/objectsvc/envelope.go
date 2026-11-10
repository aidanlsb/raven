package objectsvc

import (
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
