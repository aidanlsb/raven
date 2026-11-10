package commandimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleDelete executes the canonical `delete` command.
func HandleDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	references := commandIDsArg(req.Args, "references")
	stdinMode := boolArg(req.Args, "stdin") || len(references) > 0

	sess, failure := openSession(vaultPath, vaultsession.Config())
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	vaultCfg := rt.VaultCfg

	if stdinMode {
		if len(references) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe object references or file paths to stdin, one per line")
		}
		return runDeleteBulk(sess, references, req.Confirm)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "reference"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires reference argument", nil, "Usage: rvn delete <reference>")
	}

	// Delete resolves the target reference (which is schema-aware) before
	// removing a file, so a corrupt schema could resolve to the wrong object.
	// Treat a schema load failure as fatal on this safety-sensitive path.
	if rt.SchemaLoadErr != nil {
		return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
	}
	deletionCfg := vaultCfg.GetDeletionConfig()
	if req.Preview {
		preview, err := objectsvc.PreviewDeleteByReference(rt, objectsvc.DeleteByReferenceRequest{
			Reference: reference,
			Behavior:  deletionCfg.Behavior,
			TrashDir:  deletionCfg.TrashDir,
		})
		if err != nil {
			return mapContentMutationError(err)
		}

		warnings := deleteBacklinkCommandWarnings(preview.Backlinks)
		return commandexec.SuccessWithWarnings(objectsvc.DeletePreviewResult{
			Preview:   true,
			ObjectID:  preview.ObjectID,
			Behavior:  preview.Behavior,
			TrashDir:  deletionCfg.TrashDir,
			Backlinks: preview.Backlinks,
		}, warnings, nil)
	}

	deleteReq := objectsvc.DeleteByReferenceRequest{
		Reference: reference,
		Behavior:  deletionCfg.Behavior,
		TrashDir:  deletionCfg.TrashDir,
	}
	serviceResult, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.DeleteByReferenceResult, mutation.ChangeSet, error) {
		result, err := objectsvc.DeleteByReference(rt, deleteReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}

	warnings := make([]commandexec.Warning, 0, 1)
	warnings = append(warnings, deleteBacklinkCommandWarnings(serviceResult.Backlinks)...)

	data := objectsvc.DeleteResult{
		Deleted:           serviceResult.ObjectID,
		Behavior:          serviceResult.Behavior,
		MissingReferences: missingRefsOf(proj),
	}
	if serviceResult.TrashPath != "" {
		relDest, relErr := filepath.Rel(vaultPath, serviceResult.TrashPath)
		if relErr == nil {
			data.TrashPath = filepath.ToSlash(relDest)
		}
	}
	warnings = appendCommandWarnings(warnings, proj.Warnings)

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runDeleteBulk(sess *vaultsession.Session, ids []string, confirm bool) commandexec.Result {
	vaultCfg := sess.Runtime().VaultCfg
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	deletionCfg := vaultCfg.GetDeletionConfig()
	request := objectsvc.DeleteBulkRequest{
		ObjectIDs: fileIDs,
		Behavior:  deletionCfg.Behavior,
		TrashDir:  deletionCfg.TrashDir,
	}

	if !confirm {
		preview, err := objectsvc.PreviewDeleteBulk(sess.Runtime(), request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("references", ids)
		}
		return commandexec.Success(objectsvc.DeleteBulkPreviewResult{
			BulkPreviewResult: objectsvc.BulkPreviewResult{
				Preview:  true,
				Action:   preview.Action,
				Items:    preview.Items,
				Skipped:  preview.Skipped,
				Total:    preview.Total,
				Warnings: warnings,
			},
			Behavior: preview.Behavior,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.DeleteBulkSummary, mutation.ChangeSet, error) {
		summary, err := objectsvc.ApplyDeleteBulk(rt, request)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return summary, summary.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure.WithAttemptedIDs("references", ids)
	}
	data := objectsvc.DeleteBulkResult{
		BulkSummaryResult: objectsvc.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             summary.Results,
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefsOf(proj),
		},
		Deleted:  summary.Deleted,
		Behavior: summary.Behavior,
	}
	allWarnings := appendCommandWarnings(warnings, proj.Warnings)
	return commandexec.SuccessWithWarnings(data, allWarnings, &commandexec.Meta{Count: summary.Total - summary.Skipped - summary.Errors})
}

func deleteBacklinkCommandWarnings(backlinks []model.Reference) []commandexec.Warning {
	if len(backlinks) == 0 {
		return nil
	}

	backlinkIDs := make([]string, 0, len(backlinks))
	for _, bl := range backlinks {
		backlinkIDs = append(backlinkIDs, bl.SourceID)
	}

	return []commandexec.Warning{{
		Code:    codes.WarnBacklinks,
		Message: fmt.Sprintf("Target is referenced by %d other objects", len(backlinks)),
		Ref:     strings.Join(backlinkIDs, ", "),
	}}
}
