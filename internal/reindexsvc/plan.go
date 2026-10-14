package reindexsvc

import (
	"fmt"
	"os"
	"path/filepath"

	ravenignore "github.com/aidanlsb/raven/internal/ignore"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
)

// filePlan is the set of dirty, deleted, and excluded paths a reindex pass
// will apply, plus the mtime/journal inputs used to select stale files.
type filePlan struct {
	pending          indexjournal.Snapshot
	dirtyPaths       map[string]struct{}
	forceFullScan    bool
	indexedMtimes    map[string]int64
	recoveryComplete bool

	excludedFiles   []string
	deletedFiles    []string
	errors          []string
	warningMessages []string
	filesExcluded   int
	filesDeleted    int
}

func (p *filePlan) shouldParse(relativePath string, fileMtime int64) bool {
	if p == nil {
		return true
	}
	if p.forceFullScan {
		return true
	}
	if _, dirty := p.dirtyPaths[relativePath]; dirty {
		return true
	}
	indexedMtime := p.indexedMtimes[relativePath]
	return indexedMtime <= 0 || fileMtime > indexedMtime
}

func planFiles(prep *preparedRun) (*filePlan, error) {
	pending, err := indexjournal.Load(prep.vaultPath)
	if err != nil {
		return nil, wrapDB(err, "failed to load index dirty journal").WithSuggestion("Run 'rvn reindex --full' after repairing or removing disposable .raven metadata")
	}
	dirtyPaths := make(map[string]struct{})
	for _, relPath := range pending.Paths() {
		dirtyPaths[relPath] = struct{}{}
	}

	plan := &filePlan{
		pending:          pending,
		dirtyPaths:       dirtyPaths,
		forceFullScan:    pending.RequiresFullScan(),
		recoveryComplete: true,
		excludedFiles:    []string{},
		deletedFiles:     []string{},
		errors:           []string{},
		warningMessages:  []string{},
	}

	if !prep.dryRun {
		trashRemoved, err := prep.db.RemoveFilesWithPrefix(".trash/")
		if err != nil {
			plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to clean up trash files from index: %v", err))
			plan.recoveryComplete = false
		}
		if trashRemoved > 0 {
			plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("Cleaned up %d files from .trash/ in index", trashRemoved))
		}
	}

	if prep.incremental {
		plan.recoveryComplete = recoverIncrementalIndex(prep.db, prep.vaultPath, pending, prep.excludeMatcher, prep.dryRun, plan)

		var mtimeErr error
		plan.indexedMtimes, mtimeErr = prep.db.GetFileMtimes()
		if mtimeErr != nil {
			// Falling back to parsing every file is safe and preserves refresh
			// behavior when the optimization query is unavailable.
			plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to load indexed file mtimes: %v", mtimeErr))
			plan.indexedMtimes = nil
		}
	}

	return plan, nil
}

func recoverIncrementalIndex(
	db *index.Database,
	vaultPath string,
	pending indexjournal.Snapshot,
	excludeMatcher *ravenignore.Matcher,
	dryRun bool,
	plan *filePlan,
) (recoveryComplete bool) {
	recoveryComplete = true

	excludedFiles, excludedErr := indexedExcludedFiles(db, excludeMatcher)
	if excludedErr != nil {
		plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to check for excluded files: %v", excludedErr))
		recoveryComplete = false
	} else {
		plan.excludedFiles = excludedFiles
		plan.filesExcluded = len(excludedFiles)
		if !dryRun {
			if removeErr := db.RemoveFiles(excludedFiles); removeErr != nil {
				plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to clean up excluded files: %v", removeErr))
				recoveryComplete = false
			}
		}
	}

	if dryRun {
		indexedPaths, indexedErr := db.AllIndexedFilePaths()
		if indexedErr != nil {
			plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to check for deleted files: %v", indexedErr))
		} else {
			for _, relPath := range indexedPaths {
				fullPath := filepath.Join(vaultPath, relPath)
				if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
					plan.deletedFiles = append(plan.deletedFiles, relPath)
				}
			}
			plan.filesDeleted = len(plan.deletedFiles)
		}
	} else {
		deletedFiles, delErr := db.RemoveDeletedFiles(vaultPath)
		if delErr != nil {
			plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to clean up deleted files: %v", delErr))
			recoveryComplete = false
		}
		plan.deletedFiles = deletedFiles
		plan.filesDeleted = len(deletedFiles)
	}

	if !dryRun {
		for _, relPath := range pending.Paths() {
			fullPath := filepath.Join(vaultPath, filepath.FromSlash(relPath))
			_, statErr := os.Stat(fullPath)
			missing := os.IsNotExist(statErr)
			excluded := excludeMatcher.Match(relPath, false)
			if !missing && !excluded {
				continue
			}
			if statErr != nil && !missing {
				plan.errors = append(plan.errors, fmt.Sprintf("%s: %v", relPath, statErr))
				recoveryComplete = false
				continue
			}
			if removeErr := db.RemoveFile(relPath); removeErr != nil {
				plan.errors = append(plan.errors, fmt.Sprintf("%s: %v", relPath, removeErr))
				recoveryComplete = false
				continue
			}
			if clearErr := indexjournal.ClearRecoveredPath(vaultPath, pending, relPath); clearErr != nil {
				plan.warningMessages = append(plan.warningMessages, fmt.Sprintf("failed to clear recovered index path %s: %v", relPath, clearErr))
				recoveryComplete = false
			}
		}
	}

	return recoveryComplete
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
