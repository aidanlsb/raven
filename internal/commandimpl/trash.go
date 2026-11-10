package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleTrashList executes the canonical `trash list` command.
func HandleTrashList(_ context.Context, req commandexec.Request) commandexec.Result {
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.ConfigOnly())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()

	result, err := objectsvc.ListTrash(rt, objectsvc.ListTrashRequest{
		Reference: strings.TrimSpace(stringArg(req.Args, "reference")),
		Kind:      strings.TrimSpace(stringArg(req.Args, "kind")),
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
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.Required())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()

	restoreReq := objectsvc.RestoreByReferenceRequest{
		Reference: strings.TrimSpace(stringArg(req.Args, "reference")),
	}
	if req.Preview {
		result, err := objectsvc.PreviewRestoreByReference(rt, restoreReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(restoreResultData(result, true), nil)
	}

	result, proj, failure := mutateResult(sess, func(rt *vaultruntime.Runtime) (*objectsvc.RestoreByReferenceResult, mutation.ChangeSet, error) {
		result, err := objectsvc.RestoreByReference(rt, restoreReq)
		if err != nil {
			return nil, mutation.ChangeSet{}, err
		}
		return result, result.ChangeSet, nil
	})
	if failure.Error != nil {
		return failure
	}
	data := restoreResultData(result, false)
	missingReferences := missingRefsOf(proj)
	if missingReferences.MissingRefs > 0 {
		data["missing_refs"] = missingReferences.MissingRefs
		data["missing_ref_items"] = missingReferences.MissingRefItems
	}
	return commandexec.SuccessWithWarnings(data, proj.Warnings, nil)
}

// HandleTrashEmpty executes the canonical `trash empty` command.
func HandleTrashEmpty(_ context.Context, req commandexec.Request) commandexec.Result {
	sess, failure := openSession(strings.TrimSpace(req.VaultPath), vaultsession.ConfigOnly())
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()

	emptyReq := objectsvc.EmptyTrashRequest{
		OlderThan: strings.TrimSpace(stringArg(req.Args, "older-than")),
	}
	if req.Preview {
		result, err := objectsvc.PreviewEmptyTrash(rt, emptyReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(emptyTrashResultData(result, true), &commandexec.Meta{Count: len(result.Entries)})
	}

	result, err := objectsvc.EmptyTrash(rt, emptyReq)
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
