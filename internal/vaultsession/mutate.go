package vaultsession

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func mutate(rt *vaultruntime.Runtime, fn func(*vaultruntime.Runtime) (mutation.ChangeSet, error)) (MutationResult, error) {
	operationID, err := indexjournal.Begin(rt.VaultPath)
	if err != nil {
		return MutationResult{}, mapJournalBeginError(err)
	}
	changes, fnErr := fn(rt)
	if fnErr != nil {
		result := MutationResult{}
		if abandonErr := indexjournal.Abandon(rt.VaultPath, operationID); abandonErr != nil {
			result.Warnings = []commandexec.Warning{{
				Code:    codes.WarnIndexUpdateFailed,
				Message: fmt.Sprintf("failed to release index recovery guard after mutation failure: %v", abandonErr),
				Ref:     "Run 'rvn reindex' before relying on index-backed results",
			}}
		}
		return result, fnErr
	}
	return projectChangeSet(rt, changes, operationID), nil
}
