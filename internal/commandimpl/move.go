package commandimpl

import (
	"context"
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleMove executes the canonical `move` command.
func HandleMove(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0

	// Move resolves references and rewrites backlinks using the schema, so
	// require a valid schema on this safety-sensitive path.
	sess, failure := openSession(vaultPath, vaultsession.Required())
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("object_ids", objectIDs)
		}
		return failure
	}
	defer sess.Close()

	if stdinMode {
		destination := strings.TrimSpace(stringArg(req.Args, "destination"))
		if destination == "" {
			destination = strings.TrimSpace(stringArg(req.Args, "source"))
		}
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object IDs provided via stdin", nil, "Provide object IDs when using bulk move")
		}
		return runMoveBulk(sess, objectIDs, destination, boolArgDefault(req.Args, "update-refs", true), req.Confirm)
	}

	source := strings.TrimSpace(stringArg(req.Args, "source"))
	destination := strings.TrimSpace(stringArg(req.Args, "destination"))
	if source == "" || destination == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires source and destination arguments", nil, "Usage: rvn move <source> <destination>")
	}

	moveReq := objectsvc.MoveByReferenceRequest{
		Reference:     source,
		Destination:   destination,
		UpdateRefs:    boolArgDefault(req.Args, "update-refs", true),
		SkipTypeCheck: boolArg(req.Args, "skip-type-check"),
		Preview:       req.Preview,
	}
	if req.Preview {
		serviceResult, err := objectsvc.MoveByReference(sess.Runtime(), moveReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return shapeMoveResult(serviceResult, req.Preview, vaultsession.MutationResult{})
	}

	moveReq.Preview = false
	serviceResult, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.MoveByReferenceResult, mutation.ChangeSet, error) {
		result, err := objectsvc.MoveByReference(rt, moveReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}
	return shapeMoveResult(serviceResult, false, proj)
}

func shapeMoveResult(serviceResult *objectsvc.MoveByReferenceResult, preview bool, proj vaultsession.MutationResult) commandexec.Result {
	if serviceResult.NeedsConfirm && serviceResult.TypeMismatch != nil {
		mismatch := serviceResult.TypeMismatch
		// Nothing was written: the move is blocked pending confirmation, so
		// report a preview phase regardless of the normalized apply resolution.
		return commandexec.SuccessWithWarnings(commandpayload.MoveConfirmationResult{
			Source:       serviceResult.SourceID,
			Destination:  serviceResult.DestinationID,
			Preview:      preview,
			NeedsConfirm: true,
			Reason:       serviceResult.Reason,
		}, appendCommandWarnings([]commandexec.Warning{{
			Code: codes.WarnTypeMismatch,
			Message: fmt.Sprintf("Moving to '%s/' which is the default directory for type '%s', but file has type '%s'",
				mismatch.DestinationDir, mismatch.ExpectedType, mismatch.ActualType),
			Ref: fmt.Sprintf("Use --skip-type-check to proceed, or change the file's type to '%s'", mismatch.ExpectedType),
		}}, proj.Warnings), nil).WithMutationPhase(commandexec.MutationPhasePreview)
	}

	warnings := warningMessagesToCommandWarnings(serviceResult.WarningMessages, indexUpdateFailedWarningCode)
	data := commandpayload.MoveResult{
		Source:      serviceResult.SourceID,
		Destination: serviceResult.DestinationID,
	}
	if preview {
		data.Preview = true
		data.Status = "preview"
	}
	if len(serviceResult.UpdatedRefs) > 0 {
		data.UpdatedRefs = serviceResult.UpdatedRefs
	}
	if len(serviceResult.UpdatedRefFields) > 0 {
		data.UpdatedRefFields = canonicalMoveRefFieldUpdates(serviceResult.UpdatedRefFields)
	}
	if !preview {
		data.MissingReferences = missingRefsOf(proj)
		warnings = appendCommandWarnings(warnings, proj.Warnings)
	}

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func canonicalMoveRefFieldUpdates(updates []objectsvc.RefFieldUpdate) []commandpayload.MoveRefFieldUpdate {
	items := make([]commandpayload.MoveRefFieldUpdate, 0, len(updates))
	for _, update := range updates {
		items = append(items, commandpayload.MoveRefFieldUpdate{
			SourceID: update.SourceID,
			File:     update.FilePath,
			Field:    update.Field,
		})
	}
	return items
}

func runMoveBulk(sess *vaultsession.Session, ids []string, destination string, updateRefs bool, confirm bool) commandexec.Result {
	if strings.TrimSpace(destination) == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "no destination provided", nil, "Usage: rvn move --stdin <destination-directory/>").
			WithAttemptedIDs("object_ids", ids)
	}
	if !strings.HasSuffix(destination, "/") {
		return commandexec.Failure("INVALID_INPUT", "destination must be a directory (end with /)", nil, "Example: rvn move --stdin archive/projects/").
			WithAttemptedIDs("object_ids", ids)
	}

	fileIDs, sectionIDs := splitSectionIDs(ids)
	if len(sectionIDs) > 0 {
		return moveSectionSourceFailure(sectionIDs).WithAttemptedIDs("object_ids", ids)
	}
	request := objectsvc.MoveBulkRequest{
		ObjectIDs:      fileIDs,
		DestinationDir: destination,
		UpdateRefs:     updateRefs,
	}

	if !confirm {
		preview, err := objectsvc.PreviewMoveBulk(sess.Runtime(), request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("object_ids", ids)
		}
		return commandexec.Success(commandpayload.MoveBulkPreviewResult{
			BulkPreviewResult: commandpayload.BulkPreviewResult{
				Preview: true,
				Action:  preview.Action,
				Items:   canonicalBulkPreviewItems(preview.Items),
				Skipped: canonicalBulkResults(preview.Skipped),
				Total:   preview.Total,
			},
			Destination: preview.Destination,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.MoveBulkSummary, mutation.ChangeSet, error) {
		summary, err := objectsvc.ApplyMoveBulk(rt, request)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return summary, summary.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure.WithAttemptedIDs("object_ids", ids)
	}

	data := commandpayload.MoveBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalBulkResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefsOf(proj),
		},
		Moved:       summary.Moved,
		Destination: summary.Destination,
	}
	allWarnings := appendCommandWarnings(
		warningMessagesToCommandWarnings(summary.WarningMessages, indexUpdateFailedWarningCode),
		proj.Warnings,
	)
	return commandexec.SuccessWithWarnings(data, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func moveSectionSourceFailure(sectionIDs []string) commandexec.Result {
	return commandexec.Failure(
		codes.ErrInvalidInput,
		"rvn move does not accept section sources",
		map[string]interface{}{"section_ids": sectionIDs},
		`Use 'rvn section move <file#section>' to reorder/reparent, or 'rvn section rename <file#section> "<new heading text>"' to change heading identity`,
	)
}
