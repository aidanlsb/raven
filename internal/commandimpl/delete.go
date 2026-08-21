package commandimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleDelete executes the canonical `delete` command.
func HandleDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)

	references := commandIDsArg(req.Args, "references")
	stdinMode := boolArg(req.Args, "stdin") || len(references) > 0

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		if stdinMode {
			return failure.WithAttemptedIDs("references", references)
		}
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg

	if stdinMode {
		if len(references) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no references provided via stdin", nil, "Pipe object references or file paths to stdin, one per line")
		}
		return runDeleteBulk(rt, references, req.Confirm, req.IndexJournalOperation)
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
	sch := rt.Schema
	deletionCfg := vaultCfg.GetDeletionConfig()
	if req.Preview {
		preview, err := objectsvc.PreviewDeleteByReference(objectsvc.DeleteByReferenceRequest{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
			Schema:      sch,
			Reference:   reference,
			Behavior:    deletionCfg.Behavior,
			TrashDir:    deletionCfg.TrashDir,
			Runtime:     rt,
		})
		if err != nil {
			return mapContentMutationError(err)
		}

		warnings := deleteBacklinkCommandWarnings(preview.Backlinks)
		return commandexec.SuccessWithWarnings(commandpayload.DeletePreviewResult{
			Preview:   true,
			ObjectID:  preview.ObjectID,
			Behavior:  preview.Behavior,
			TrashDir:  deletionCfg.TrashDir,
			Backlinks: preview.Backlinks,
		}, warnings, nil)
	}

	serviceResult, err := objectsvc.DeleteByReference(objectsvc.DeleteByReferenceRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		Schema:      sch,
		Reference:   reference,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
		Runtime:     rt,
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	warnings := make([]commandexec.Warning, 0, 1)
	warnings = append(warnings, deleteBacklinkCommandWarnings(serviceResult.Backlinks)...)

	data := commandpayload.DeleteResult{
		Deleted:  serviceResult.ObjectID,
		Behavior: serviceResult.Behavior,
	}
	if serviceResult.TrashPath != "" {
		relDest, relErr := filepath.Rel(vaultPath, serviceResult.TrashPath)
		if relErr == nil {
			data.TrashPath = filepath.ToSlash(relDest)
		}
	}
	missingRefs, postWarnings := applyChangeSet(rt, serviceResult.ChangeSet, req.IndexJournalOperation)
	data.MissingReferences = missingRefs
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runDeleteBulk(rt *vaultruntime.Runtime, ids []string, confirm bool, journalOperation string) commandexec.Result {
	vaultPath := rt.VaultPath
	vaultCfg := rt.VaultCfg
	fileIDs, sectionIDs := splitSectionIDs(ids)
	warnings := sectionSkipWarnings(sectionIDs)
	deletionCfg := vaultCfg.GetDeletionConfig()
	request := objectsvc.DeleteBulkRequest{
		VaultPath:   vaultPath,
		VaultConfig: vaultCfg,
		ObjectIDs:   fileIDs,
		Behavior:    deletionCfg.Behavior,
		TrashDir:    deletionCfg.TrashDir,
		Runtime:     rt,
	}

	if !confirm {
		preview, err := objectsvc.PreviewDeleteBulk(request)
		if err != nil {
			return mapContentMutationError(err).WithAttemptedIDs("references", ids)
		}
		return commandexec.Success(commandpayload.DeleteBulkPreviewResult{
			BulkPreviewResult: commandpayload.BulkPreviewResult{
				Preview: true,
				Action:  preview.Action,
				Items:   canonicalDeletePreviewItems(preview.Items),
				Skipped: canonicalDeleteResults(preview.Skipped),
				Total:   preview.Total,
			},
			Warnings: warnings,
			Behavior: preview.Behavior,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyDeleteBulk(request)
	if err != nil {
		return mapContentMutationError(err).WithAttemptedIDs("references", ids)
	}

	missingRefs, postWarnings := applyChangeSet(rt, summary.ChangeSet, journalOperation)
	data := commandpayload.DeleteBulkResult{
		BulkSummaryResult: commandpayload.BulkSummaryResult{
			OK:                summary.Errors == 0,
			Action:            summary.Action,
			Items:             canonicalDeleteResults(summary.Results),
			Total:             summary.Total,
			Skipped:           summary.Skipped,
			Errors:            summary.Errors,
			MissingReferences: missingRefs,
		},
		Deleted:  summary.Deleted,
		Behavior: summary.Behavior,
	}
	allWarnings := appendCommandWarnings(warnings, postWarnings)
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
