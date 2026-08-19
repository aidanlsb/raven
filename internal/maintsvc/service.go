package maintsvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput  Code = codes.ErrInvalidInput
	CodeDatabaseError Code = codes.ErrDatabase
)

func newError(code Code, message, suggestion string, err error) *svcerr.Error {
	return &svcerr.Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

type StatsResult struct {
	FileCount   int `json:"file_count"`
	ObjectCount int `json:"object_count"`
	TraitCount  int `json:"trait_count"`
	RefCount    int `json:"ref_count"`
}

func Stats(rt *vaultruntime.Runtime) (*StatsResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, newError(CodeInvalidInput, "vault path is required", "", err)
	}

	if err := rt.OpenDB(); err != nil {
		return nil, newError(CodeDatabaseError, "failed to open database", "Run 'rvn reindex' to rebuild the database", err)
	}

	stats, err := rt.DB.Stats()
	if err != nil {
		return nil, newError(CodeDatabaseError, "failed to query stats", "", err)
	}

	return &StatsResult{
		FileCount:   stats.FileCount,
		ObjectCount: stats.ObjectCount,
		TraitCount:  stats.TraitCount,
		RefCount:    stats.RefCount,
	}, nil
}
