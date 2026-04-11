package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/maintsvc"
	"github.com/aidanlsb/raven/internal/svcerror"
)

// HandleVaultStats executes the canonical `vault_stats` command.
func HandleVaultStats(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	stats, err := maintsvc.Stats(req.VaultPath)
	if err != nil {
		svcErr, ok := svcerror.As(err)
		if !ok {
			return commandexec.Failure("INTERNAL_ERROR", err.Error(), nil, "")
		}
		return commandexec.Failure(svcErr.Code, svcErr.Message, nil, svcErr.Suggestion)
	}

	return commandexec.Success(map[string]interface{}{
		"file_count":   stats.FileCount,
		"object_count": stats.ObjectCount,
		"trait_count":  stats.TraitCount,
		"ref_count":    stats.RefCount,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}
