package reindexsvc

import (
	"context"
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type RunRequest struct {
	Full    bool
	DryRun  bool
	Context context.Context
}

type RunResult struct {
	FilesIndexed  int
	FilesSkipped  int
	FilesDeleted  int
	FilesExcluded int
	Objects       int
	Traits        int
	References    int
	SchemaRebuilt bool
	Incremental   bool
	DryRun        bool
	Errors        []string

	StaleFiles    []string
	DeletedFiles  []string
	ExcludedFiles []string

	RefsResolved   int
	RefsUnresolved int
	HasRefResult   bool

	WarningMessages []string
}

func (r *RunResult) Data() map[string]interface{} {
	data := map[string]interface{}{
		"files_indexed":  r.FilesIndexed,
		"files_skipped":  r.FilesSkipped,
		"files_deleted":  r.FilesDeleted,
		"files_excluded": r.FilesExcluded,
		"objects":        r.Objects,
		"traits":         r.Traits,
		"references":     r.References,
		"schema_rebuilt": r.SchemaRebuilt,
		"incremental":    r.Incremental,
		"dry_run":        r.DryRun,
		"errors":         r.Errors,
	}
	if r.Incremental {
		data["stale_files"] = r.StaleFiles
		data["deleted_files"] = r.DeletedFiles
		data["excluded_files"] = r.ExcludedFiles
	}
	if r.HasRefResult {
		data["refs_resolved"] = r.RefsResolved
		data["refs_unresolved"] = r.RefsUnresolved
	}
	return data
}

func wrapDB(err error, msg string) *svcerr.Error {
	return svcerr.Wrap(codes.ErrDatabase, fmt.Sprintf("%s: %v", msg, err), err)
}

func Run(rt *vaultruntime.Runtime, req RunRequest) (*RunResult, error) {
	prep, err := prepare(rt, req)
	if err != nil {
		return nil, err
	}
	defer prep.Close()

	plan, err := planFiles(prep)
	if err != nil {
		return nil, err
	}
	return apply(prep, plan)
}
