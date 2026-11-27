package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/vaultsession"
)

// HandleVaultStats executes the canonical `vault_stats` command.
func HandleVaultStats(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	sess, failure := openSession(req.VaultPath, vaultsession.Read(refreshPolicy(req.Args)))
	if failure.Error != nil {
		return failure
	}
	defer sess.Close()
	rt := sess.Runtime()
	refreshWarnings := sess.RefreshWarnings()

	stats, err := readsvc.Stats(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return withIndexRefreshWarnings(refreshWarnings, commandexec.Success(stats.Payload(), &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()}))
}
