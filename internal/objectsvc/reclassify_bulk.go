package objectsvc

import (
	"errors"
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/fieldmutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ReclassifyBulkRequest struct {
	VaultPath   string
	VaultConfig *config.VaultConfig
	Schema      *schema.Schema
	ObjectIDs   []string

	NewTypeName string
	FieldValues map[string]schema.FieldValue

	NoMove     bool
	UpdateRefs bool
	Force      bool

	ParseOptions *parser.ParseOptions
	Runtime      *vaultruntime.Runtime
}

type ReclassifyBulkPreviewItem struct {
	ID            string   `json:"id"`
	Action        string   `json:"action"`
	ObjectID      string   `json:"object_id"`
	OldType       string   `json:"old_type"`
	NewType       string   `json:"new_type"`
	File          string   `json:"file"`
	Moved         bool     `json:"moved"`
	OldPath       string   `json:"old_path,omitempty"`
	NewPath       string   `json:"new_path,omitempty"`
	UpdatedRefs   []string `json:"updated_refs"`
	AddedFields   []string `json:"added_fields"`
	DroppedFields []string `json:"dropped_fields"`
	NeedsConfirm  bool     `json:"needs_confirm"`
	Reason        string   `json:"reason,omitempty"`
}

type ReclassifyBulkResult struct {
	ID            string                 `json:"id"`
	Status        string                 `json:"status"`
	Reason        string                 `json:"reason,omitempty"`
	ErrorCode     codes.ErrorCode        `json:"error_code,omitempty"`
	ErrorDetails  map[string]interface{} `json:"error_details,omitempty"`
	ObjectID      string                 `json:"object_id,omitempty"`
	OldType       string                 `json:"old_type,omitempty"`
	NewType       string                 `json:"new_type,omitempty"`
	File          string                 `json:"file,omitempty"`
	Moved         bool                   `json:"moved"`
	OldPath       string                 `json:"old_path,omitempty"`
	NewPath       string                 `json:"new_path,omitempty"`
	UpdatedRefs   []string               `json:"updated_refs"`
	AddedFields   []string               `json:"added_fields"`
	DroppedFields []string               `json:"dropped_fields"`
	NeedsConfirm  bool                   `json:"needs_confirm"`
}

type ReclassifyBulkPreview struct {
	Action          string
	Items           []ReclassifyBulkPreviewItem
	Skipped         []ReclassifyBulkResult
	Total           int
	NewType         string
	WarningMessages []string
}

type ReclassifyBulkSummary struct {
	Action          string
	Results         []ReclassifyBulkResult
	Total           int
	Skipped         int
	Errors          int
	Reclassified    int
	NewType         string
	WarningMessages []string
}

func PreviewReclassifyBulk(req ReclassifyBulkRequest) (*ReclassifyBulkPreview, error) {
	if err := validateReclassifyBulkRequest(req); err != nil {
		return nil, err
	}

	items := make([]ReclassifyBulkPreviewItem, 0, len(req.ObjectIDs))
	skipped := make([]ReclassifyBulkResult, 0)
	warnings := make([]string, 0)

	for _, id := range req.ObjectIDs {
		result, err := reclassifyBulkObject(req, id, true)
		if err != nil {
			skipped = append(skipped, reclassifyBulkErrorResult(id, req.NewTypeName, "skipped", err))
			continue
		}
		items = append(items, reclassifyBulkPreviewResult(id, result))
		warnings = append(warnings, result.WarningMessages...)
	}

	return &ReclassifyBulkPreview{
		Action:          "reclassify",
		Items:           items,
		Skipped:         skipped,
		Total:           len(req.ObjectIDs),
		NewType:         req.NewTypeName,
		WarningMessages: warnings,
	}, nil
}

func ApplyReclassifyBulk(req ReclassifyBulkRequest, onReclassified func(*ReclassifyResult)) (*ReclassifyBulkSummary, error) {
	if err := validateReclassifyBulkRequest(req); err != nil {
		return nil, err
	}

	results := make([]ReclassifyBulkResult, 0, len(req.ObjectIDs))
	warnings := make([]string, 0)
	reclassifiedCount := 0
	skippedCount := 0
	errorCount := 0

	for _, id := range req.ObjectIDs {
		result, err := reclassifyBulkObject(req, id, false)
		if err != nil {
			item := reclassifyBulkErrorResult(id, req.NewTypeName, "error", err)
			if item.ErrorCode == codes.ErrRefNotFound {
				item.Status = "skipped"
				skippedCount++
			} else {
				errorCount++
			}
			results = append(results, item)
			continue
		}

		item := reclassifyBulkApplyResult(id, result)
		if result.NeedsConfirm {
			item.Status = "skipped"
			skippedCount++
		} else {
			item.Status = "reclassified"
			reclassifiedCount++
			if onReclassified != nil {
				onReclassified(result)
			}
		}
		results = append(results, item)
		warnings = append(warnings, result.WarningMessages...)
	}

	return &ReclassifyBulkSummary{
		Action:          "reclassify",
		Results:         results,
		Total:           len(results),
		Skipped:         skippedCount,
		Errors:          errorCount,
		Reclassified:    reclassifiedCount,
		NewType:         req.NewTypeName,
		WarningMessages: warnings,
	}, nil
}

func validateReclassifyBulkRequest(req ReclassifyBulkRequest) error {
	if req.VaultConfig == nil {
		return newError(ErrorValidationFailed, "vault config is required", "Fix raven.yaml and try again", nil, nil)
	}
	if req.Schema == nil {
		return newError(ErrorValidationFailed, "schema is required", "Fix schema.yaml and try again", nil, nil)
	}
	if req.NewTypeName == "" {
		return newError(ErrorInvalidInput, "new type is required", "Usage: rvn reclassify <new-type> --stdin", nil, nil)
	}
	return nil
}

func reclassifyBulkObject(req ReclassifyBulkRequest, id string, preview bool) (*ReclassifyResult, error) {
	return ReclassifyByReference(ReclassifyByReferenceRequest{
		VaultPath:    req.VaultPath,
		VaultConfig:  req.VaultConfig,
		Schema:       req.Schema,
		Reference:    id,
		NewTypeName:  req.NewTypeName,
		FieldValues:  req.FieldValues,
		NoMove:       req.NoMove,
		UpdateRefs:   req.UpdateRefs,
		Force:        req.Force,
		Preview:      preview,
		ParseOptions: req.ParseOptions,
		Runtime:      req.Runtime,
	})
}

func reclassifyBulkPreviewResult(id string, result *ReclassifyResult) ReclassifyBulkPreviewItem {
	return ReclassifyBulkPreviewItem{
		ID:            id,
		Action:        "reclassify",
		ObjectID:      result.ObjectID,
		OldType:       result.OldType,
		NewType:       result.NewType,
		File:          result.File,
		Moved:         result.Moved,
		OldPath:       result.OldPath,
		NewPath:       result.NewPath,
		UpdatedRefs:   nonNilReclassifyStrings(result.UpdatedRefs),
		AddedFields:   nonNilReclassifyStrings(result.AddedFields),
		DroppedFields: nonNilReclassifyStrings(result.DroppedFields),
		NeedsConfirm:  result.NeedsConfirm,
		Reason:        result.Reason,
	}
}

func reclassifyBulkApplyResult(id string, result *ReclassifyResult) ReclassifyBulkResult {
	return ReclassifyBulkResult{
		ID:            id,
		Reason:        result.Reason,
		ObjectID:      result.ObjectID,
		OldType:       result.OldType,
		NewType:       result.NewType,
		File:          result.File,
		Moved:         result.Moved,
		OldPath:       result.OldPath,
		NewPath:       result.NewPath,
		UpdatedRefs:   nonNilReclassifyStrings(result.UpdatedRefs),
		AddedFields:   nonNilReclassifyStrings(result.AddedFields),
		DroppedFields: nonNilReclassifyStrings(result.DroppedFields),
		NeedsConfirm:  result.NeedsConfirm,
	}
}

func reclassifyBulkErrorResult(id, newType, status string, err error) ReclassifyBulkResult {
	result := ReclassifyBulkResult{
		ID:            id,
		Status:        status,
		Reason:        err.Error(),
		NewType:       newType,
		UpdatedRefs:   []string{},
		AddedFields:   []string{},
		DroppedFields: []string{},
	}

	if svcErr, ok := svcerr.AsError(err); ok {
		result.ErrorCode = svcErr.Code
		result.Reason = svcErr.Message
		result.ErrorDetails = svcErr.Details
		if oldType, ok := svcErr.Details["old_type"].(string); ok {
			result.OldType = oldType
		}
		return result
	}

	var unknownErr *fieldmutation.UnknownFieldMutationError
	if errors.As(err, &unknownErr) {
		result.ErrorCode = codes.ErrUnknownField
		result.ErrorDetails = unknownErr.Details()
		return result
	}

	var validationErr *fieldmutation.ValidationError
	if errors.As(err, &validationErr) {
		result.ErrorCode = codes.ErrValidationFailed
		return result
	}

	result.ErrorCode = codes.ErrInternal
	result.Reason = fmt.Sprintf("reclassify failed: %v", err)
	return result
}

func nonNilReclassifyStrings(values []string) []string {
	return append([]string{}, values...)
}
