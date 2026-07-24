package commandimpl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// applyChangeSet coordinates derived post-write work for an applied mutation.
// Markdown files remain the durable source of truth: projection failures are
// returned as warnings and never turn a successful write into a failed command.
func applyChangeSet(rt *vaultruntime.Runtime, changes mutation.ChangeSet, journalOperations ...string) (map[string]interface{}, []commandexec.Warning) {
	if rt == nil {
		return nil, nil
	}
	journalOperation := ""
	if len(journalOperations) > 0 {
		journalOperation = journalOperations[0]
	}

	indexPaths := existingIndexPaths(rt, changes)
	affectedPaths := uniquePostMutationPaths(changes.RemovedPaths(), indexPaths)
	var warnings []commandexec.Warning
	trackedOperation, journalErr := indexjournal.SetPaths(rt.VaultPath, journalOperation, affectedPaths)
	if journalErr != nil {
		warnings = append(warnings, indexJournalWarning("failed to record pending index updates", journalErr))
		trackedOperation = ""
	}
	if changes.Empty() {
		return nil, warnings
	}

	autoReindexEnabled := rt.VaultCfg != nil && rt.VaultCfg.IsAutoReindexEnabled()
	failedPaths := make(map[string]struct{})
	if autoReindexEnabled {
		canProject := true
		if err := rt.OpenDB(); err != nil {
			warnings = append(warnings, indexUpdateWarning(
				rt.VaultPath,
				filepath.Join(rt.VaultPath, ".raven", "index.db"),
				"failed to open index database",
				err,
			))
			canProject = false
		}
		var projectionLock *indexjournal.ProjectionLock
		if canProject {
			var err error
			projectionLock, err = indexjournal.LockProjection(rt.VaultPath)
			if err != nil {
				warnings = append(warnings, indexJournalWarning("failed to lock index projection", err))
				canProject = false
			}
		}

		if !canProject {
			for _, relPath := range affectedPaths {
				failedPaths[relPath] = struct{}{}
			}
		} else {
			removedPaths, currentIndexPaths := currentPostMutationPlan(rt, changes)
			indexPaths = currentIndexPaths
			for _, relPath := range removedPaths {
				if warning, ok := removeIndexPathWarning(rt, relPath); ok {
					warnings = append(warnings, warning)
					failedPaths[relPath] = struct{}{}
				}
			}
			for _, relPath := range indexPaths {
				if warning, ok := projectIndexPathWarning(rt, relPath); ok {
					warnings = append(warnings, warning)
					failedPaths[relPath] = struct{}{}
				}
			}
			successfulPaths := make([]string, 0, len(affectedPaths))
			for _, relPath := range affectedPaths {
				if _, failed := failedPaths[relPath]; !failed {
					successfulPaths = append(successfulPaths, relPath)
				}
			}
			if err := indexjournal.ClearPaths(rt.VaultPath, trackedOperation, successfulPaths...); err != nil {
				warnings = append(warnings, indexJournalWarning("failed to clear completed index updates", err))
			}
		}
		if projectionLock != nil {
			if err := projectionLock.Close(); err != nil {
				warnings = append(warnings, indexJournalWarning("failed to unlock index projection", err))
			}
		}
	}

	// Missing-reference detection relies on a current resolver. If projection
	// failed, or indexing was intentionally deferred, stale IDs can produce
	// false REF_TARGET_MISSING remediation for files that exist on disk.
	if len(warnings) > 0 || !autoReindexEnabled {
		return nil, warnings
	}

	missingPaths := make([]string, 0, len(indexPaths))
	for _, relPath := range indexPaths {
		if paths.HasMDExtension(relPath) {
			missingPaths = append(missingPaths, relPath)
		}
	}
	missingData, missingWarnings := missingRefEnvelope(rt, missingPaths...)
	return missingData, appendCommandWarnings(warnings, missingWarnings)
}

func uniquePostMutationPaths(groups ...[]string) []string {
	seen := make(map[string]struct{})
	var result []string
	for _, group := range groups {
		for _, relPath := range group {
			if _, ok := seen[relPath]; ok {
				continue
			}
			seen[relPath] = struct{}{}
			result = append(result, relPath)
		}
	}
	return result
}

// currentPostMutationPlan rechecks removed paths while projection is
// serialized. A newer concurrent mutation may have recreated a deleted/moved
// path after this ChangeSet was produced; project that current file instead of
// letting an older removal erase its newer index row.
func currentPostMutationPlan(rt *vaultruntime.Runtime, changes mutation.ChangeSet) (removedPaths, indexPaths []string) {
	indexPaths = existingIndexPaths(rt, changes)
	indexed := make(map[string]struct{}, len(indexPaths))
	for _, relPath := range indexPaths {
		indexed[relPath] = struct{}{}
	}
	for _, relPath := range changes.RemovedPaths() {
		filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
		if _, err := os.Stat(filePath); err == nil || !os.IsNotExist(err) {
			if _, exists := indexed[relPath]; !exists {
				indexPaths = append(indexPaths, relPath)
				indexed[relPath] = struct{}{}
			}
			continue
		}
		removedPaths = append(removedPaths, relPath)
	}
	return removedPaths, indexPaths
}

func indexJournalWarning(message string, err error) commandexec.Warning {
	return commandexec.Warning{
		Code:    indexUpdateFailedWarningCode,
		Message: fmt.Sprintf("%s: %v", message, err),
		Ref:     indexUpdateFailedWarningRef,
	}
}

func existingIndexPaths(rt *vaultruntime.Runtime, changes mutation.ChangeSet) []string {
	removed := make(map[string]struct{}, len(changes.Deleted)+len(changes.Moved))
	for _, relPath := range changes.RemovedPaths() {
		removed[relPath] = struct{}{}
	}

	candidates := changes.IndexPaths()
	result := make([]string, 0, len(candidates))
	for _, relPath := range candidates {
		if _, wasRemoved := removed[relPath]; wasRemoved {
			filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
			if _, err := os.Stat(filePath); os.IsNotExist(err) {
				continue
			}
		}
		result = append(result, relPath)
	}
	return result
}

func removeIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (commandexec.Warning, bool) {
	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to open index database", err), true
	}
	if err := rt.DB.RemoveFile(relPath); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to remove file from index", err), true
	}
	return commandexec.Warning{}, false
}

func projectIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (commandexec.Warning, bool) {
	filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
	if paths.HasMDExtension(relPath) {
		return autoReindexWarningLocked(rt, filePath)
	}

	if rt.VaultCfg == nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to index asset", fmt.Errorf("vault config is required")), true
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to read file", err), true
	}
	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to open index database", err), true
	}
	asset := vault.BuildAsset(relPath, info, rt.VaultCfg)
	if err := rt.DB.IndexAsset(asset); err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to update index", err), true
	}
	return commandexec.Warning{}, false
}
