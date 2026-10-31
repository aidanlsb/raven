package readsvc

import (
	"errors"
	"fmt"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// ShouldRefresh reports whether an index-backed read should reindex stale
// files first.
//
// Default is on when auto_reindex is enabled (itself default true).
// noRefresh always skips. An explicit refresh pointer overrides the config, so
// query --refresh still forces a refresh when auto_reindex is false.
func ShouldRefresh(rt *vaultruntime.Runtime, noRefresh bool, refresh *bool) bool {
	if noRefresh {
		return false
	}
	if refresh != nil {
		return *refresh
	}
	if rt == nil || rt.VaultCfg == nil {
		return true
	}
	return rt.VaultCfg.IsAutoReindexEnabled()
}

// CanRefresh reports whether the runtime has the config and schema needed to
// incrementally refresh or fully rebuild the index.
func CanRefresh(rt *vaultruntime.Runtime) bool {
	return rt != nil && rt.VaultCfg != nil && rt.Schema != nil && rt.SchemaLoadErr == nil
}

// EnsureIndexFresh incrementally reindexes stale, new, and deleted Markdown
// files using SmartReindex. If the index is missing, schema-mismatched, or a
// rebuild was interrupted, it falls back to a full reindexsvc rebuild and
// reopens the runtime database.
func EnsureIndexFresh(rt *vaultruntime.Runtime) (SmartReindexReport, error) {
	if rt == nil {
		return SmartReindexReport{}, fmt.Errorf("runtime is required")
	}
	needsRebuild, err := indexNeedsRebuild(rt)
	if err != nil {
		return SmartReindexReport{}, err
	}
	if needsRebuild {
		return rebuildIndex(rt)
	}
	return SmartReindex(rt)
}

func indexNeedsRebuild(rt *vaultruntime.Runtime) (bool, error) {
	if rt.DB == nil {
		err := rt.OpenDB()
		if err == nil {
			return incompatibleOpenIndex(rt)
		}
		if isRebuildRequired(err) {
			return true, nil
		}
		return false, err
	}
	return incompatibleOpenIndex(rt)
}

func incompatibleOpenIndex(rt *vaultruntime.Runtime) (bool, error) {
	if rt.DB == nil {
		return false, fmt.Errorf("runtime with database is required")
	}
	compatible, err := rt.DB.SchemaCompatible()
	if err != nil {
		return false, err
	}
	return !compatible, nil
}

func isRebuildRequired(err error) bool {
	if errors.Is(err, index.ErrIndexRebuildRequired) {
		return true
	}
	var setupErr *vaultruntime.SetupError
	return errors.As(err, &setupErr) && setupErr.Failure == vaultruntime.SetupFailureIndexRebuildRequired
}

func rebuildIndex(rt *vaultruntime.Runtime) (SmartReindexReport, error) {
	rt.Close()
	result, err := reindexsvc.Run(rt, reindexsvc.RunRequest{Full: true})
	if err != nil {
		return SmartReindexReport{}, err
	}
	if err := rt.OpenDB(); err != nil {
		return SmartReindexReport{}, err
	}
	report := SmartReindexReport{Indexed: result.FilesIndexed}
	for _, msg := range result.Errors {
		report.Failures = append(report.Failures, SmartReindexFailure{
			Stage:  "index",
			ErrMsg: msg,
		})
	}
	for _, msg := range result.WarningMessages {
		report.Warnings = append(report.Warnings, SmartReindexWarning{Message: msg})
	}
	return report, nil
}
