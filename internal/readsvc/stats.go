package readsvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type StatsResult struct {
	FileCount   int
	ObjectCount int
	TraitCount  int
	RefCount    int
}

func Stats(rt *vaultruntime.Runtime) (*StatsResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if err := rt.OpenDB(); err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open database", err).
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}
	stats, err := rt.DB.Stats()
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to query stats", err)
	}
	return &StatsResult{
		FileCount: stats.FileCount, ObjectCount: stats.ObjectCount,
		TraitCount: stats.TraitCount, RefCount: stats.RefCount,
	}, nil
}
