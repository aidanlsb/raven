package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
)

// HandleTrashList executes the canonical `trash list` command.
func HandleTrashList(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newConfigOnlyCommandVaultRuntime(strings.TrimSpace(req.VaultPath))
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	result, err := objectsvc.ListTrash(objectsvc.ListTrashRequest{
		VaultPath:   rt.VaultPath,
		VaultConfig: rt.VaultCfg,
		Reference:   strings.TrimSpace(stringArg(req.Args, "reference")),
		Kind:        strings.TrimSpace(stringArg(req.Args, "kind")),
	})
	if err != nil {
		return mapContentMutationError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"items":             result.Entries,
		"total":             len(result.Entries),
		"trash_dir":         result.TrashDir,
		"deletion_behavior": result.DeletionBehavior,
	}, &commandexec.Meta{Count: len(result.Entries)})
}

// HandleRestore executes the canonical `restore` command.
func HandleRestore(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newRequiredCommandVaultRuntime(strings.TrimSpace(req.VaultPath), false)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	restoreReq := objectsvc.RestoreByReferenceRequest{
		VaultPath:   rt.VaultPath,
		VaultConfig: rt.VaultCfg,
		Reference:   strings.TrimSpace(stringArg(req.Args, "reference")),
		Runtime:     rt,
	}
	if req.Preview {
		result, err := objectsvc.PreviewRestoreByReference(restoreReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(restoreResultData(result, true), nil)
	}

	result, err := objectsvc.RestoreByReference(restoreReq)
	if err != nil {
		return mapContentMutationError(err)
	}
	data := restoreResultData(result, false)
	missingReferences, warnings := applyChangeSet(rt, result.ChangeSet, req.IndexJournalOperation)
	if missingReferences.MissingRefs > 0 {
		data["missing_refs"] = missingReferences.MissingRefs
		data["missing_ref_items"] = missingReferences.MissingRefItems
	}
	return commandexec.SuccessWithWarnings(data, warnings, nil)
}

func restoreResultData(result *objectsvc.RestoreByReferenceResult, preview bool) map[string]interface{} {
	data := map[string]interface{}{
		"preview":      preview,
		"reference":    result.Entry.Reference,
		"trash_path":   result.Entry.TrashPath,
		"restore_path": result.Entry.RestorePath,
		"kind":         result.Entry.Kind,
	}
	if !preview {
		data["restored"] = result.Entry.Reference
	}
	return data
}
