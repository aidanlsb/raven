package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/maintsvc"
)

// HandleVaultStats executes the canonical `vault_stats` command.
func HandleVaultStats(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	rt, failure := newDatabaseCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	stats, err := maintsvc.Stats(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"file_count":   stats.FileCount,
		"object_count": stats.ObjectCount,
		"trait_count":  stats.TraitCount,
		"ref_count":    stats.RefCount,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}
