package reindexsvc

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
)

type Code = codes.ErrorCode

const (
	CodeInvalidInput  Code = codes.ErrInvalidInput
	CodeSchemaInvalid Code = codes.ErrSchemaInvalid
	CodeConfigInvalid Code = codes.ErrConfigInvalid
	CodeDatabaseError Code = codes.ErrDatabase
	CodeFileReadError Code = codes.ErrFileRead
	CodeInternal      Code = codes.ErrInternal
)

func newError(code Code, message, suggestion string, err error) *svcerr.Error {
	return &svcerr.Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

type RunRequest struct {
	VaultPath string
	Full      bool
	DryRun    bool
	Context   context.Context
}

type RunResult struct {
	FilesIndexed  int
	FilesSkipped  int
	FilesDeleted  int
	FilesExcluded int
	Objects       int
	Traits        int
	References    int
	Assets        int
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
		"assets":         r.Assets,
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

func Run(req RunRequest) (*RunResult, error) {
	vaultPath := strings.TrimSpace(req.VaultPath)
	if vaultPath == "" {
		return nil, newError(CodeInvalidInput, "vault path is required", "", nil)
	}

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return nil, newError(CodeSchemaInvalid, fmt.Sprintf("failed to load schema: %v", err), "Run 'rvn init' to create a schema", err)
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, newError(CodeConfigInvalid, fmt.Sprintf("failed to load raven.yaml: %v", err), "Fix raven.yaml and try again", err)
	}
	if vaultCfg == nil {
		vaultCfg = &config.VaultConfig{}
	}

	rebuildSession, err := index.OpenWithRebuild(vaultPath, index.RebuildOptions{DryRun: req.DryRun})
	if err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to open database: %v", err), "Run 'rvn reindex' to rebuild the database", err)
	}
	defer rebuildSession.Close()

	db := rebuildSession.Database()
	wasRebuilt := rebuildSession.SchemaRebuilt()

	incremental := !req.Full
	if wasRebuilt {
		incremental = false
	}

	if !incremental && !req.DryRun {
		if err := rebuildSession.BeginFullRebuild(); err != nil {
			return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to mark index for full reindex: %v", err), "", err)
		}
		if err := db.ClearAllData(); err != nil {
			return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to clear database for full reindex: %v", err), "", err)
		}
	}

	dailyDir := vaultCfg.GetDailyDirectory()
	if dailyDir == "" {
		dailyDir = "daily"
	}
	db.SetDailyDirectory(dailyDir)
	if !req.DryRun {
		// Bulk reindex always does a full resolver pass after indexing the walk set.
		// Avoid rebuilding whole-vault resolver state once per file on the hot path.
		db.SetAutoResolveRefs(false)
	}

	parseOpts := parseopts.FromVaultConfig(vaultCfg)
	excludeMatcher, err := ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
	if err != nil {
		return nil, newError(CodeConfigInvalid, fmt.Sprintf("invalid exclude config: %v", err), "Fix raven.yaml exclude patterns and try again", err)
	}

	result := &RunResult{
		SchemaRebuilt:   wasRebuilt,
		Incremental:     incremental,
		DryRun:          req.DryRun,
		Errors:          []string{},
		StaleFiles:      []string{},
		DeletedFiles:    []string{},
		ExcludedFiles:   []string{},
		WarningMessages: []string{},
		HasRefResult:    false,
		RefsResolved:    0,
		RefsUnresolved:  0,
		FilesIndexed:    0,
		FilesSkipped:    0,
		FilesDeleted:    0,
		FilesExcluded:   0,
		Objects:         0,
		Traits:          0,
		References:      0,
		Assets:          0,
	}
	dryRunFileStats := make(map[string]index.IndexStats)
	dryRunAssetFiles := make(map[string]struct{})
	dryRunStats := index.IndexStats{}

	if !req.DryRun {
		trashRemoved, err := db.RemoveFilesWithPrefix(".trash/")
		if err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up trash files from index: %v", err))
		}
		if trashRemoved > 0 {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Cleaned up %d files from .trash/ in index", trashRemoved))
		}
	}

	if incremental {
		excludedFiles, excludedErr := indexedExcludedFiles(db, excludeMatcher)
		if excludedErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to check for excluded files: %v", excludedErr))
		} else {
			result.ExcludedFiles = excludedFiles
			result.FilesExcluded = len(excludedFiles)
			if !req.DryRun {
				if removeErr := db.RemoveFiles(excludedFiles); removeErr != nil {
					result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up excluded files: %v", removeErr))
				}
			}
		}

		if req.DryRun {
			indexedPaths, indexedErr := db.AllIndexedFilePaths()
			if indexedErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to check for deleted files: %v", indexedErr))
			} else {
				for _, relPath := range indexedPaths {
					fullPath := filepath.Join(vaultPath, relPath)
					if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
						result.DeletedFiles = append(result.DeletedFiles, relPath)
					}
				}
				result.FilesDeleted = len(result.DeletedFiles)
			}
		} else {
			deletedFiles, delErr := db.RemoveDeletedFiles(vaultPath)
			if delErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up deleted files: %v", delErr))
			}
			result.DeletedFiles = deletedFiles
			result.FilesDeleted = len(deletedFiles)
		}
	}

	walkOpts := &vault.WalkOptions{ParseOptions: parseOpts, ExcludeMatcher: excludeMatcher}
	walkErr := vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(walkResult vault.WalkResult) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkResult.Error != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, walkResult.Error))
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}

		if incremental {
			indexedMtime, mtimeErr := db.GetFileMtime(walkResult.RelativePath)
			if mtimeErr == nil && indexedMtime > 0 && walkResult.FileMtime <= indexedMtime {
				result.FilesSkipped++
				return nil
			}
			result.StaleFiles = append(result.StaleFiles, walkResult.RelativePath)
		}

		if req.DryRun {
			result.FilesIndexed++
			docStats := parsedDocumentStats(walkResult.Document)
			dryRunFileStats[walkResult.RelativePath] = docStats
			dryRunStats.ObjectCount += docStats.ObjectCount
			dryRunStats.TraitCount += docStats.TraitCount
			dryRunStats.RefCount += docStats.RefCount
			return nil
		}

		if idxErr := db.IndexDocumentWithMtime(walkResult.Document, sch, walkResult.FileMtime); idxErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, idxErr))
			return nil
		}

		result.WarningMessages = append(result.WarningMessages, index.UnknownFrontmatterWarnings(walkResult.Document, sch)...)

		result.FilesIndexed++
		return nil
	})
	if walkErr != nil {
		return nil, newError(CodeFileReadError, fmt.Sprintf("error walking vault: %v", walkErr), "", walkErr)
	}

	assetWalkErr := vault.WalkAssetFilesWithOptions(vaultPath, vaultCfg, &vault.AssetWalkOptions{ExcludeMatcher: excludeMatcher}, func(walkResult vault.AssetWalkResult) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkResult.Error != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, walkResult.Error))
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}
		if walkResult.Asset == nil {
			return nil
		}

		if incremental {
			// Asset metadata depends on raven.yaml kind/default-path rules, so
			// unchanged file mtimes do not prove the indexed row is fresh.
			result.StaleFiles = append(result.StaleFiles, walkResult.RelativePath)
		}

		if req.DryRun {
			result.FilesIndexed++
			result.Assets++
			dryRunAssetFiles[walkResult.RelativePath] = struct{}{}
			return nil
		}
		if idxErr := db.IndexAsset(walkResult.Asset); idxErr != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, idxErr))
			return nil
		}
		result.FilesIndexed++
		return nil
	})
	if assetWalkErr != nil {
		return nil, newError(CodeFileReadError, fmt.Sprintf("error walking asset files: %v", assetWalkErr), "", assetWalkErr)
	}

	if !req.DryRun && !incremental && len(result.Errors) > 0 {
		err := fmt.Errorf("%d file(s) failed during full reindex; first failure: %s", len(result.Errors), result.Errors[0])
		return nil, newError(
			CodeFileReadError,
			err.Error(),
			"Fix invalid or unreadable vault files and run 'rvn reindex --full' again",
			err,
		)
	}

	if req.DryRun {
		if incremental {
			removedFiles := uniqueStrings(result.DeletedFiles, result.ExcludedFiles)
			projected, err := projectedDryRunStats(db, removedFiles, dryRunFileStats)
			if err != nil {
				return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to project dry-run stats: %v", err), "", err)
			}
			result.Objects = projected.ObjectCount
			result.Traits = projected.TraitCount
			result.References = projected.RefCount
			assetCount, err := projectedDryRunAssetCount(db, removedFiles, dryRunAssetFiles)
			if err != nil {
				return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to project dry-run asset stats: %v", err), "", err)
			}
			result.Assets = assetCount
		} else {
			result.Objects = dryRunStats.ObjectCount
			result.Traits = dryRunStats.TraitCount
			result.References = dryRunStats.RefCount
		}
		return result, nil
	}

	if !req.DryRun {
		// Run the resolve pass even when no files were reindexed: earlier
		// writes may have added targets for refs that are still recorded as
		// unresolved, and the pass short-circuits cheaply when nothing is
		// unresolved.
		refResult, refErr := db.ResolveReferencesWithSchema(dailyDir, sch)
		if refErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to resolve references: %v", refErr))
		} else if refResult != nil {
			result.RefsResolved = refResult.Resolved
			result.RefsUnresolved = refResult.Unresolved
			result.HasRefResult = true
		}

		if result.FilesIndexed > 0 {
			if analyzeErr := db.Analyze(); analyzeErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to analyze database: %v", analyzeErr))
			}
		}
	}

	stats, err := db.Stats()
	if err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to get stats: %v", err), "", err)
	}
	result.Objects = stats.ObjectCount
	result.Traits = stats.TraitCount
	result.References = stats.RefCount
	result.Assets = stats.AssetCount

	if err := rebuildSession.Complete(); err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to complete index rebuild: %v", err), "Run 'rvn reindex --full' to rebuild the database", err)
	}

	return result, nil
}

func parsedDocumentStats(doc *parser.ParsedDocument) index.IndexStats {
	if doc == nil {
		return index.IndexStats{}
	}
	return index.IndexStats{
		ObjectCount: len(doc.Objects),
		TraitCount:  len(doc.Traits),
		RefCount:    len(doc.Refs),
	}
}

func projectedDryRunStats(db *index.Database, deletedFiles []string, reindexedFiles map[string]index.IndexStats) (*index.IndexStats, error) {
	stats, err := db.Stats()
	if err != nil {
		return nil, err
	}
	projected := *stats

	for _, filePath := range deletedFiles {
		current, err := fileIndexStats(db, filePath)
		if err != nil {
			return nil, err
		}
		projected.ObjectCount -= current.ObjectCount
		projected.TraitCount -= current.TraitCount
		projected.RefCount -= current.RefCount
	}

	for filePath, next := range reindexedFiles {
		current, err := fileIndexStats(db, filePath)
		if err != nil {
			return nil, err
		}
		projected.ObjectCount += next.ObjectCount - current.ObjectCount
		projected.TraitCount += next.TraitCount - current.TraitCount
		projected.RefCount += next.RefCount - current.RefCount
	}

	return &projected, nil
}

func projectedDryRunAssetCount(db *index.Database, deletedFiles []string, reindexedAssets map[string]struct{}) (int, error) {
	assets, err := db.QueryAssets()
	if err != nil {
		return 0, err
	}
	current := make(map[string]struct{}, len(assets))
	for _, asset := range assets {
		current[asset.FilePath] = struct{}{}
	}
	for _, filePath := range deletedFiles {
		delete(current, filePath)
	}
	for filePath := range reindexedAssets {
		current[filePath] = struct{}{}
	}
	return len(current), nil
}

func indexedExcludedFiles(db *index.Database, matcher *ravenignore.Matcher) ([]string, error) {
	if matcher == nil {
		return nil, nil
	}
	indexedPaths, err := db.AllIndexedFilePaths()
	if err != nil {
		return nil, err
	}
	excluded := make([]string, 0)
	for _, relPath := range indexedPaths {
		if matcher.Match(relPath, false) {
			excluded = append(excluded, relPath)
		}
	}
	return excluded, nil
}

func uniqueStrings(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
			if _, ok := seen[value]; ok {
				continue
			}
			seen[value] = struct{}{}
			out = append(out, value)
		}
	}
	return out
}

func fileIndexStats(db *index.Database, filePath string) (index.IndexStats, error) {
	if db == nil {
		return index.IndexStats{}, fmt.Errorf("database is nil")
	}

	var stats index.IndexStats
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM objects WHERE file_path = ?`, filePath).Scan(&stats.ObjectCount); err != nil {
		return index.IndexStats{}, err
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM traits WHERE file_path = ?`, filePath).Scan(&stats.TraitCount); err != nil {
		return index.IndexStats{}, err
	}
	if err := db.DB().QueryRow(`SELECT COUNT(*) FROM refs WHERE file_path = ?`, filePath).Scan(&stats.RefCount); err != nil {
		return index.IndexStats{}, err
	}
	return stats, nil
}
