package commandimpl

import (
	"context"
	"time"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// HandleVaultStats executes the canonical `vault_stats` command.
func HandleVaultStats(_ context.Context, req commandexec.Request) commandexec.Result {
	start := time.Now()

	rt, failure := newDatabaseCommandVaultRuntime(req.VaultPath)
	if failure.Error != nil {
		return failure
	}
	defer rt.Close()

	stats, err := vaultStats(rt)
	if err != nil {
		return commandexec.FromServiceError(err)
	}

	return commandexec.Success(map[string]interface{}{
		"file_count":   stats.fileCount,
		"object_count": stats.objectCount,
		"trait_count":  stats.traitCount,
		"ref_count":    stats.refCount,
	}, &commandexec.Meta{QueryTimeMs: time.Since(start).Milliseconds()})
}

type statsResult struct {
	fileCount   int
	objectCount int
	traitCount  int
	refCount    int
}

func vaultStats(rt *vaultruntime.Runtime) (*statsResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}

	if err := rt.OpenDB(); err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}

	stats, err := rt.DB.Stats()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to query stats", err)
	}

	return &statsResult{
		fileCount:   stats.FileCount,
		objectCount: stats.ObjectCount,
		traitCount:  stats.TraitCount,
		refCount:    stats.RefCount,
	}, nil
}
