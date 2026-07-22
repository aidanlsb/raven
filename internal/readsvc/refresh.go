package readsvc

import (
	"fmt"

	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

func CheckStaleness(rt *Runtime) (bool, []string, error) {
	if rt == nil || rt.DB == nil {
		return false, nil, fmt.Errorf("runtime with database is required")
	}
	staleness, err := rt.DB.CheckStaleness(rt.VaultPath)
	if err != nil {
		return false, nil, err
	}
	matcher, err := excludeMatcher(rt)
	if err != nil {
		return false, nil, err
	}
	staleFiles := filterIncluded(staleness.StaleFiles, matcher)
	return len(staleFiles) > 0, staleFiles, nil
}

// SmartReindexFailure records a file that could not be refreshed.
type SmartReindexFailure struct {
	Path   string
	Stage  string // "parse" or "index"
	ErrMsg string
}

// SmartReindexWarning records a non-fatal issue observed while refreshing.
type SmartReindexWarning struct {
	Path    string
	Message string
}

// SmartReindexReport summarizes an incremental refresh.
type SmartReindexReport struct {
	Indexed  int
	Failures []SmartReindexFailure
	Warnings []SmartReindexWarning
}

func SmartReindex(rt *Runtime) (SmartReindexReport, error) {
	if rt == nil || rt.DB == nil {
		return SmartReindexReport{}, fmt.Errorf("runtime with database is required")
	}

	if rt.VaultCfg == nil {
		return SmartReindexReport{}, fmt.Errorf("runtime with vault config is required")
	}
	vaultCfg := rt.VaultCfg
	rt.DB.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	if rt.SchemaLoadErr != nil && rt.Schema == nil {
		return SmartReindexReport{}, rt.SchemaLoadErr
	}
	if rt.Schema == nil {
		return SmartReindexReport{}, fmt.Errorf("runtime with schema is required")
	}
	sch := rt.Schema

	if _, err := rt.DB.RemoveDeletedFiles(rt.VaultPath); err != nil {
		return SmartReindexReport{}, err
	}
	matcher, err := excludeMatcher(rt)
	if err != nil {
		return SmartReindexReport{}, err
	}
	if err := removeExcludedIndexedFiles(rt, matcher); err != nil {
		return SmartReindexReport{}, err
	}

	indexedMtimes, err := rt.DB.GetFileMtimes()
	if err != nil {
		// The mtime lookup is only an optimization. Parse all files if it fails
		// so refresh retains its existing best-effort behavior.
		indexedMtimes = nil
	}

	walkOpts := &vault.WalkOptions{
		ParseOptions:   rt.ParseOptions,
		ExcludeMatcher: matcher,
		ShouldParse: func(relativePath string, fileMtime int64) bool {
			indexedMtime := indexedMtimes[relativePath]
			return indexedMtime <= 0 || fileMtime > indexedMtime
		},
	}
	report := SmartReindexReport{}
	err = vault.WalkMarkdownFilesWithOptions(rt.VaultPath, walkOpts, func(result vault.WalkResult) error {
		if result.ParseSkipped {
			return nil
		}

		if result.Error != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path:   result.RelativePath,
				Stage:  "parse",
				ErrMsg: result.Error.Error(),
			})
			return nil //nolint:nilerr // record and continue; caller surfaces Failures
		}

		if err := rt.DB.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); err != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path:   result.RelativePath,
				Stage:  "index",
				ErrMsg: err.Error(),
			})
			return nil //nolint:nilerr // record and continue; caller surfaces Failures
		}

		for _, warning := range index.UnknownFrontmatterWarnings(result.Document, sch) {
			report.Warnings = append(report.Warnings, SmartReindexWarning{
				Path:    result.RelativePath,
				Message: warning,
			})
		}

		report.Indexed++
		return nil
	})
	if err != nil {
		return report, err
	}

	return report, nil
}

func excludeMatcher(rt *Runtime) (*ravenignore.Matcher, error) {
	if rt == nil {
		return ravenignore.NewMatcher(nil)
	}
	if rt.VaultCfg == nil {
		return nil, fmt.Errorf("runtime with vault config is required")
	}
	return ravenignore.NewMatcher(rt.VaultCfg.GetExcludePatterns())
}

func removeExcludedIndexedFiles(rt *Runtime, matcher *ravenignore.Matcher) error {
	indexedPaths, err := rt.DB.AllIndexedFilePaths()
	if err != nil {
		return err
	}
	var excluded []string
	for _, relPath := range indexedPaths {
		if matcher.Match(relPath, false) {
			excluded = append(excluded, relPath)
		}
	}
	return rt.DB.RemoveFiles(excluded)
}

func filterIncluded(paths []string, matcher *ravenignore.Matcher) []string {
	if len(paths) == 0 {
		return nil
	}
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		if matcher.Match(path, false) {
			continue
		}
		out = append(out, path)
	}
	return out
}
