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

// HandleTrashEmpty executes the canonical `trash empty` command.
func HandleTrashEmpty(_ context.Context, req commandexec.Request) commandexec.Result {
	rt, failure := newConfigOnlyCommandVaultRuntime(strings.TrimSpace(req.VaultPath))
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	emptyReq := objectsvc.EmptyTrashRequest{
		VaultPath:   rt.VaultPath,
		VaultConfig: rt.VaultCfg,
		OlderThan:   strings.TrimSpace(stringArg(req.Args, "older-than")),
	}
	if req.Preview {
		result, err := objectsvc.PreviewEmptyTrash(emptyReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(emptyTrashResultData(result, true), &commandexec.Meta{Count: len(result.Entries)})
	}

	result, err := objectsvc.EmptyTrash(emptyReq)
	if err != nil {
		return mapContentMutationError(err)
	}
	return commandexec.Success(emptyTrashResultData(result, false), &commandexec.Meta{Count: len(result.Entries)})
}

func emptyTrashResultData(result *objectsvc.EmptyTrashResult, preview bool) map[string]interface{} {
	data := map[string]interface{}{
		"preview":   preview,
		"items":     result.Entries,
		"total":     len(result.Entries),
		"trash_dir": result.TrashDir,
	}
	if result.OlderThan != "" {
		data["older_than"] = result.OlderThan
	}
	if !preview {
		data["removed"] = len(result.Entries)
	}
	return data
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
