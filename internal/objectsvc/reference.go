package objectsvc

import (
	"errors"
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

func resolveReferenceForMutation(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, reference string) (*readsvc.ResolveResult, error) {
	rt := &readsvc.Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
		Schema:    sch,
	}

	resolved, err := readsvc.ResolveReference(reference, rt, false)
	if err != nil {
		var ambiguousErr *readsvc.AmbiguousRefError
		if errors.As(err, &ambiguousErr) {
			return nil, newError(
				ErrorRefAmbiguous,
				ambiguousErr.Error(),
				"Use a full object ID/path to disambiguate",
				map[string]interface{}{"matches": ambiguousErr.Matches},
				err,
			)
		}
		var notFoundErr *readsvc.RefNotFoundError
		if errors.As(err, &notFoundErr) {
			return nil, newError(
				ErrorRefNotFound,
				notFoundErr.Error(),
				"Check the object reference and run 'rvn reindex' if needed",
				nil,
				err,
			)
		}
		return nil, newError(
			ErrorUnexpected,
			fmt.Sprintf("failed to resolve object reference: %v", err),
			"Check the object reference and run 'rvn reindex' if needed",
			nil,
			err,
		)
	}

	return resolved, nil
}
