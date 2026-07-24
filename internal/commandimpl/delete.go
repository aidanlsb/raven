package commandimpl

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleDelete executes the canonical `delete` command.
func HandleDelete(_ context.Context, req commandexec.Request) commandexec.Result {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
	}

	rt, failure := newConfigCommandVaultRuntime(vaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()
	vaultCfg := rt.VaultCfg

	objectIDs := commandIDsArg(req.Args, "object_ids")
	stdinMode := boolArg(req.Args, "stdin") || len(objectIDs) > 0
	if stdinMode {
		if len(objectIDs) == 0 {
			return commandexec.Failure("MISSING_ARGUMENT", "no object or asset IDs provided via stdin", nil, "Pipe object or asset IDs to stdin, one per line")
		}
		return runDeleteBulk(rt, objectIDs, req.Confirm)
	}

	reference := strings.TrimSpace(stringArg(req.Args, "object_id"))
	if reference == "" {
		return commandexec.Failure("MISSING_ARGUMENT", "requires object or asset ID argument", nil, "Usage: rvn delete <object-or-asset-id>")
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
		return commandexec.SuccessWithWarnings(map[string]interface{}{
			"preview":   true,
			"object_id": preview.ObjectID,
			"behavior":  preview.Behavior,
			"trash_dir": deletionCfg.TrashDir,
			"backlinks": preview.Backlinks,
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

	data := map[string]interface{}{
		"deleted":  serviceResult.ObjectID,
		"behavior": serviceResult.Behavior,
	}
	if serviceResult.TrashPath != "" {
		relDest, relErr := filepath.Rel(vaultPath, serviceResult.TrashPath)
		if relErr == nil {
			data["trash_path"] = filepath.ToSlash(relDest)
		}
	}
	postData, postWarnings := applyChangeSet(rt, serviceResult.ChangeSet)
	data = mergeDataFields(data, postData)
	warnings = appendCommandWarnings(warnings, postWarnings)

	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func runDeleteBulk(rt *vaultruntime.Runtime, ids []string, confirm bool) commandexec.Result {
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
			return mapContentMutationError(err)
		}
		return commandexec.Success(map[string]interface{}{
			"preview":  true,
			"action":   preview.Action,
			"items":    canonicalDeletePreviewItems(preview.Items),
			"skipped":  canonicalDeleteResults(preview.Skipped),
			"total":    preview.Total,
			"warnings": warnings,
			"behavior": preview.Behavior,
		}, &commandexec.Meta{Count: len(preview.Items)})
	}

	summary, err := objectsvc.ApplyDeleteBulk(request)
	if err != nil {
		return mapContentMutationError(err)
	}

	data := map[string]interface{}{
		"ok":       summary.Errors == 0,
		"action":   summary.Action,
		"items":    canonicalDeleteResults(summary.Results),
		"total":    summary.Total,
		"skipped":  summary.Skipped,
		"errors":   summary.Errors,
		"deleted":  summary.Deleted,
		"behavior": summary.Behavior,
	}
	postData, postWarnings := applyChangeSet(rt, summary.ChangeSet)
	data = mergeDataFields(data, postData)
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
