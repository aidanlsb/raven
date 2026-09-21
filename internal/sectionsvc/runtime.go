// Package sectionsvc implements mutations on Markdown-derived sections.
package sectionsvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func requireSectionRuntime(rt *vaultruntime.Runtime) error {
	if err := vaultruntime.Require(rt); err != nil {
		return svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.VaultCfg == nil {
		return svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	return nil
}

// lockSectionMutation acquires the projection lock shared by create/move/delete/rename.
// Preview skips the lock. The returned function always releases it and is safe to defer.
func lockSectionMutation(rt *vaultruntime.Runtime, preview bool) (func(), error) {
	if err := requireSectionRuntime(rt); err != nil {
		return nil, err
	}
	lock, err := reindexsvc.LockProjection(rt, preview)
	if err != nil {
		return nil, err
	}
	if lock == nil {
		return func() {}, nil
	}
	return func() { _ = lock.Close() }, nil
}

func normalizeMutationError(err error) error {
	if _, ok := svcerr.AsError(err); ok {
		return err
	}
	return svcerr.Wrap(codes.ErrInternal, err.Error(), err)
}
