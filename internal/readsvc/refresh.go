package readsvc

import (
	"fmt"
	"os"
	"path/filepath"

	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/paths"
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
	Stage  string // "parse", "read", "index", or "journal"
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

	pending, err := indexjournal.Load(rt.VaultPath)
	if err != nil {
		return SmartReindexReport{}, fmt.Errorf("load index dirty journal: %w", err)
	}
	report := SmartReindexReport{}

	matcher, err := excludeMatcher(rt)
	if err != nil {
		return SmartReindexReport{}, err
	}
	if err := recoverRemovedDirtyPaths(rt, pending, matcher); err != nil {
		return SmartReindexReport{}, err
	}
	if _, err := rt.DB.RemoveDeletedFiles(rt.VaultPath); err != nil {
		return SmartReindexReport{}, err
	}
	if err := removeExcludedIndexedFiles(rt, matcher); err != nil {
		return SmartReindexReport{}, err
	}

	dirtyPaths := make(map[string]struct{})
	for _, relPath := range pending.Paths() {
		dirtyPaths[relPath] = struct{}{}
	}
	forceFullScan := pending.RequiresFullScan()
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
			if forceFullScan {
				return true
			}
			if _, dirty := dirtyPaths[relativePath]; dirty {
				return true
			}
			indexedMtime := indexedMtimes[relativePath]
			return indexedMtime <= 0 || fileMtime > indexedMtime
		},
	}
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
		if err := indexjournal.ClearRecoveredPath(rt.VaultPath, pending, result.RelativePath); err != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path:   result.RelativePath,
				Stage:  "journal",
				ErrMsg: err.Error(),
			})
		}
		return nil
	})
	if err != nil {
		return report, err
	}

	if err := recoverDirtyAssets(rt, pending, matcher, forceFullScan, &report); err != nil {
		return report, err
	}
	if forceFullScan && len(report.Failures) == 0 {
		if err := indexjournal.CompleteRecoveredUnknown(rt.VaultPath, pending); err != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path:   filepath.ToSlash(filepath.Join(".raven", indexjournal.Filename)),
				Stage:  "journal",
				ErrMsg: err.Error(),
			})
		}
	}

	return report, nil
}

func recoverRemovedDirtyPaths(rt *Runtime, pending indexjournal.Snapshot, matcher *ravenignore.Matcher) error {
	for _, relPath := range pending.Paths() {
		fullPath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
		_, statErr := os.Stat(fullPath)
		missing := os.IsNotExist(statErr)
		excluded := matcher.Match(relPath, false)
		if !missing && !excluded {
			continue
		}
		if statErr != nil && !missing {
			return fmt.Errorf("inspect pending index path %s: %w", relPath, statErr)
		}
		if err := rt.DB.RemoveFile(relPath); err != nil {
			return fmt.Errorf("remove pending index path %s: %w", relPath, err)
		}
		if err := indexjournal.ClearRecoveredPath(rt.VaultPath, pending, relPath); err != nil {
			return fmt.Errorf("clear pending index path %s: %w", relPath, err)
		}
	}
	return nil
}

func recoverDirtyAssets(
	rt *Runtime,
	pending indexjournal.Snapshot,
	matcher *ravenignore.Matcher,
	forceFullScan bool,
	report *SmartReindexReport,
) error {
	knownAssets := make(map[string]struct{})
	for _, relPath := range pending.Paths() {
		if !paths.HasMDExtension(relPath) && !matcher.Match(relPath, false) {
			knownAssets[relPath] = struct{}{}
		}
	}
	if !forceFullScan {
		for relPath := range knownAssets {
			fullPath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
			info, err := os.Stat(fullPath)
			if err != nil {
				if os.IsNotExist(err) {
					continue
				}
				report.Failures = append(report.Failures, SmartReindexFailure{Path: relPath, Stage: "read", ErrMsg: err.Error()})
				continue
			}
			asset := vault.BuildAsset(relPath, info, rt.VaultCfg)
			if err := rt.DB.IndexAsset(asset); err != nil {
				report.Failures = append(report.Failures, SmartReindexFailure{Path: relPath, Stage: "index", ErrMsg: err.Error()})
				continue
			}
			report.Indexed++
			if err := indexjournal.ClearRecoveredPath(rt.VaultPath, pending, relPath); err != nil {
				report.Failures = append(report.Failures, SmartReindexFailure{Path: relPath, Stage: "journal", ErrMsg: err.Error()})
			}
		}
		return nil
	}

	return vault.WalkAssetFilesWithOptions(rt.VaultPath, rt.VaultCfg, &vault.AssetWalkOptions{ExcludeMatcher: matcher}, func(result vault.AssetWalkResult) error {
		if result.Error != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path: result.RelativePath, Stage: "read", ErrMsg: result.Error.Error(),
			})
			return nil
		}
		if result.Asset == nil {
			return nil
		}
		if err := rt.DB.IndexAsset(result.Asset); err != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path: result.RelativePath, Stage: "index", ErrMsg: err.Error(),
			})
			return nil
		}
		report.Indexed++
		if err := indexjournal.ClearRecoveredPath(rt.VaultPath, pending, result.RelativePath); err != nil {
			report.Failures = append(report.Failures, SmartReindexFailure{
				Path: result.RelativePath, Stage: "journal", ErrMsg: err.Error(),
			})
		}
		return nil
	})
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
