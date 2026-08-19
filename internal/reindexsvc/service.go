package reindexsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
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

func Run(rt *vaultruntime.Runtime, req RunRequest) (*RunResult, error) {
	if err := vaultruntime.Require(rt); err != nil {
		message := "vault path is required"
		if rt == nil {
			message = "vault runtime is required"
		}
		return nil, newError(CodeInvalidInput, message, "", err)
	}
	vaultPath := strings.TrimSpace(rt.VaultPath)

	ctx := req.Context
	if ctx == nil {
		ctx = context.Background()
	}

	if rt.SchemaLoadErr != nil {
		return nil, newError(CodeSchemaInvalid, fmt.Sprintf("failed to load schema: %v", rt.SchemaLoadErr), "Run 'rvn init' to create a schema", rt.SchemaLoadErr)
	}
	if rt.Schema == nil {
		return nil, newError(CodeSchemaInvalid, "failed to load schema: schema runtime is required", "Run 'rvn init' to create a schema", nil)
	}
	sch := rt.Schema
	vaultCfg := rt.VaultCfg
	if vaultCfg == nil {
		return nil, newError(CodeConfigInvalid, "failed to load raven.yaml: vault config runtime is required", "Fix raven.yaml and try again", nil)
	}

	db, rebuildSession, wasRebuilt, err := openRunDatabase(vaultPath, req.Full, req.DryRun)
	if err != nil {
		suggestion := "Run 'rvn reindex' to rebuild the database"
		if errors.Is(err, index.ErrIndexLocked) {
			suggestion = "Another process (such as rvn lsp) is holding the index; stop the LSP or wait for the other process to finish, then try again"
		}
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to open database: %v", err), suggestion, err)
	}
	if rebuildSession != nil {
		defer rebuildSession.Close()
	} else {
		defer db.Close()
	}

	incremental := !req.Full
	if wasRebuilt {
		incremental = false
	}

	if !incremental && !req.DryRun {
		if rebuildSession == nil {
			err := errors.New("full reindex requires an exclusive rebuild session")
			return nil, newError(CodeInternal, err.Error(), "", err)
		}
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

	excludeMatcher, err := ravenignore.NewMatcher(vaultCfg.GetExcludePatterns())
	if err != nil {
		return nil, newError(CodeConfigInvalid, fmt.Sprintf("invalid exclude config: %v", err), "Fix raven.yaml exclude patterns and try again", err)
	}
	var projectionLock *indexjournal.ProjectionLock
	if !req.DryRun {
		projectionLock, err = indexjournal.LockProjection(vaultPath)
		if err != nil {
			return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to lock index projection: %v", err), "Wait for the active write or refresh to finish, then retry", err)
		}
		defer func() { _ = projectionLock.Close() }()
	}
	pending, err := indexjournal.Load(vaultPath)
	if err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to load index dirty journal: %v", err), "Run 'rvn reindex --full' after repairing or removing disposable .raven metadata", err)
	}
	dirtyPaths := make(map[string]struct{})
	for _, relPath := range pending.Paths() {
		dirtyPaths[relPath] = struct{}{}
	}
	forceFullScan := pending.RequiresFullScan()

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
	}
	dryRunFileStats := make(map[string]index.IndexStats)
	dryRunStats := index.IndexStats{}
	recoveryComplete := true

	if !req.DryRun {
		trashRemoved, err := db.RemoveFilesWithPrefix(".trash/")
		if err != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up trash files from index: %v", err))
			recoveryComplete = false
		}
		if trashRemoved > 0 {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Cleaned up %d files from .trash/ in index", trashRemoved))
		}
	}

	if incremental {
		excludedFiles, excludedErr := indexedExcludedFiles(db, excludeMatcher)
		if excludedErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to check for excluded files: %v", excludedErr))
			recoveryComplete = false
		} else {
			result.ExcludedFiles = excludedFiles
			result.FilesExcluded = len(excludedFiles)
			if !req.DryRun {
				if removeErr := db.RemoveFiles(excludedFiles); removeErr != nil {
					result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up excluded files: %v", removeErr))
					recoveryComplete = false
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
				recoveryComplete = false
			}
			result.DeletedFiles = deletedFiles
			result.FilesDeleted = len(deletedFiles)
		}

		if !req.DryRun {
			for _, relPath := range pending.Paths() {
				fullPath := filepath.Join(vaultPath, filepath.FromSlash(relPath))
				_, statErr := os.Stat(fullPath)
				missing := os.IsNotExist(statErr)
				excluded := excludeMatcher.Match(relPath, false)
				if !missing && !excluded {
					continue
				}
				if statErr != nil && !missing {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", relPath, statErr))
					recoveryComplete = false
					continue
				}
				if removeErr := db.RemoveFile(relPath); removeErr != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", relPath, removeErr))
					recoveryComplete = false
					continue
				}
				if clearErr := indexjournal.ClearRecoveredPath(vaultPath, pending, relPath); clearErr != nil {
					result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index path %s: %v", relPath, clearErr))
					recoveryComplete = false
				}
			}
		}
	}

	var indexedMtimes map[string]int64
	if incremental {
		var mtimeErr error
		indexedMtimes, mtimeErr = db.GetFileMtimes()
		if mtimeErr != nil {
			// Falling back to parsing every file is safe and preserves refresh
			// behavior when the optimization query is unavailable.
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to load indexed file mtimes: %v", mtimeErr))
			indexedMtimes = nil
		}
	}

	walkOpts := &vault.WalkOptions{ParseOptions: rt.ParseOptions, ExcludeMatcher: excludeMatcher}
	if incremental {
		walkOpts.ShouldParse = func(relativePath string, fileMtime int64) bool {
			if forceFullScan {
				return true
			}
			if _, dirty := dirtyPaths[relativePath]; dirty {
				return true
			}
			indexedMtime := indexedMtimes[relativePath]
			return indexedMtime <= 0 || fileMtime > indexedMtime
		}
	}
	walkErr := vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(walkResult vault.WalkResult) error {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if walkResult.ParseSkipped {
			result.FilesSkipped++
			return nil
		}

		if walkResult.Error != nil {
			result.Errors = append(result.Errors, fmt.Sprintf("%s: %v", walkResult.RelativePath, walkResult.Error))
			return nil //nolint:nilerr // keep walking to collect all per-file errors
		}

		if incremental {
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
		if incremental {
			if clearErr := indexjournal.ClearRecoveredPath(vaultPath, pending, walkResult.RelativePath); clearErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index path %s: %v", walkResult.RelativePath, clearErr))
				recoveryComplete = false
			}
		}
		return nil
	})
	if walkErr != nil {
		return nil, newError(CodeFileReadError, fmt.Sprintf("error walking vault: %v", walkErr), "", walkErr)
	}

	if incremental && !req.DryRun {
		for _, relPath := range pending.Paths() {
			if paths.HasMDExtension(relPath) {
				continue
			}
			if clearErr := indexjournal.ClearRecoveredPath(vaultPath, pending, relPath); clearErr != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear unindexed file path %s: %v", relPath, clearErr))
				recoveryComplete = false
			}
		}
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

	if rebuildSession != nil {
		if err := rebuildSession.Complete(); err != nil {
			return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to complete index rebuild: %v", err), "Run 'rvn reindex --full' to rebuild the database", err)
		}
	}
	if !req.DryRun {
		switch {
		case !incremental:
			if err := indexjournal.CompleteRecoveredSnapshot(vaultPath, pending); err != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index journal: %v", err))
			}
		case forceFullScan && recoveryComplete && len(result.Errors) == 0:
			if err := indexjournal.CompleteRecoveredUnknown(vaultPath, pending); err != nil {
				result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clear recovered index journal: %v", err))
			}
		}
	}

	return result, nil
}

func openRunDatabase(vaultPath string, full, dryRun bool) (*index.Database, *index.RebuildSession, bool, error) {
	if !full {
		db, err := index.Open(vaultPath)
		if err == nil {
			return db, nil, false, nil
		}
		if !errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, nil, false, err
		}
	}

	session, err := index.OpenWithRebuild(vaultPath, index.RebuildOptions{DryRun: dryRun})
	if err != nil {
		return nil, nil, false, err
	}
	return session.Database(), session, session.SchemaRebuilt(), nil
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
		current, err := db.StatsForFile(filePath)
		if err != nil {
			return nil, err
		}
		projected.ObjectCount -= current.ObjectCount
		projected.TraitCount -= current.TraitCount
		projected.RefCount -= current.RefCount
	}

	for filePath, next := range reindexedFiles {
		current, err := db.StatsForFile(filePath)
		if err != nil {
			return nil, err
		}
		projected.ObjectCount += next.ObjectCount - current.ObjectCount
		projected.TraitCount += next.TraitCount - current.TraitCount
		projected.RefCount += next.RefCount - current.RefCount
	}

	return &projected, nil
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
