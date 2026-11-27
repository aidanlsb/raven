package commandimpl

import (
	"context"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
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

	payload := result.Payload()
	return commandexec.Success(payload, &commandexec.Meta{Count: payload.Total})
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
		Preview:   req.Preview,
	}
	if req.Preview {
		result, err := objectsvc.RestoreByReference(rt, restoreReq)
		if err != nil {
			return mapContentMutationError(err)
		}
		return commandexec.Success(result.Payload(true, check.MissingReferences{}), nil)
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
	return commandexec.SuccessWithWarnings(result.Payload(false, missingRefsOf(proj)), proj.Warnings, nil)
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
		Preview:   req.Preview,
	}
	result, err := objectsvc.EmptyTrash(rt, emptyReq)
	if err != nil {
		return mapContentMutationError(err)
	}
	return commandexec.Success(result.Payload(req.Preview), &commandexec.Meta{Count: len(result.Entries)})
}
