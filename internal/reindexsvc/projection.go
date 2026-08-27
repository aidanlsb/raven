package reindexsvc

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/indexjournal"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const IndexUpdateFailedWarningRef = "The write succeeded, but the derived index may be stale. Run 'rvn reindex' to refresh it."

// ProjectionWarning describes a non-fatal failure while updating the derived
// index after a durable file mutation.
type ProjectionWarning struct {
	Code    codes.WarningCode
	Message string
	Ref     string
	Err     error
}

// ProjectionResult contains the derived effects of projecting a ChangeSet.
// MissingRefs is populated only when every index update succeeded, because
// detection against a stale resolver could otherwise report false positives.
type ProjectionResult struct {
	MissingRefs []*check.MissingRef
	Warnings    []ProjectionWarning
}

// ProjectionLock serializes derived index projection and recovery.
type ProjectionLock struct {
	lock *indexjournal.ProjectionLock
}

// Close releases the projection lock.
func (l *ProjectionLock) Close() error {
	if l == nil || l.lock == nil {
		return nil
	}
	return l.lock.Close()
}

// LockProjection opens the runtime index and acquires the projection lock.
// A skipped lock is used by preview-only operations that do not write files.
func LockProjection(rt *vaultruntime.Runtime, skip bool) (*ProjectionLock, error) {
	if skip {
		return nil, nil
	}
	if err := rt.OpenDB(); err != nil {
		if errors.Is(err, index.ErrIndexRebuildRequired) {
			return nil, svcerr.Wrap(codes.ErrDatabaseVersion, "index schema is stale or a rebuild was interrupted", err).
				WithSuggestion("Run 'rvn reindex --full' to rebuild the index")
		}
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to open index database", err).
			WithSuggestion("Run 'rvn reindex' to rebuild the database")
	}
	lock, err := indexjournal.LockProjection(rt.VaultPath)
	if err != nil {
		return nil, svcerr.Wrap(codes.ErrDatabase, "failed to lock index projection", err).
			WithSuggestion("Wait for the active write or refresh to finish, then retry")
	}
	return &ProjectionLock{lock: lock}, nil
}

// ProjectChanges coordinates derived post-write work for an applied mutation.
// Markdown files remain the durable source of truth: projection failures are
// returned as warnings and never turn a successful write into a failed command.
func ProjectChanges(rt *vaultruntime.Runtime, changes mutation.ChangeSet, journalOperation string) ProjectionResult {
	if rt == nil {
		return ProjectionResult{}
	}

	indexPaths := existingIndexPaths(rt, changes)
	affectedPaths := uniqueProjectionPaths(changes.RemovedPaths(), indexPaths)
	var warnings []ProjectionWarning
	trackedOperation, journalErr := indexjournal.SetPaths(rt.VaultPath, journalOperation, affectedPaths)
	if journalErr != nil {
		warnings = append(warnings, indexJournalWarning("failed to record pending index updates", journalErr))
		trackedOperation = ""
	}
	if changes.Empty() {
		return ProjectionResult{Warnings: warnings}
	}

	autoReindexEnabled := rt.VaultCfg != nil && rt.VaultCfg.IsAutoReindexEnabled()
	failedPaths := make(map[string]struct{})
	requiresFullScan := false
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
			removedPaths, currentIndexPaths := currentProjectionPlan(rt, changes)
			indexPaths = currentIndexPaths
			for _, relPath := range removedPaths {
				if warning, ok := removeIndexPathWarning(rt, relPath); ok {
					warnings = append(warnings, warning)
					failedPaths[relPath] = struct{}{}
				}
			}
			for _, relPath := range indexPaths {
				if warning, projectionErr := projectIndexPathWarning(rt, relPath); projectionErr != nil {
					warnings = append(warnings, warning)
					failedPaths[relPath] = struct{}{}
					var resolutionErr *index.PostCommitReferenceResolutionError
					if errors.As(projectionErr, &resolutionErr) && resolutionErr.VaultWide {
						requiresFullScan = true
					}
				}
			}
			if requiresFullScan {
				var err error
				trackedOperation, err = indexjournal.RequireFullScan(rt.VaultPath, trackedOperation)
				if err != nil {
					warnings = append(warnings, indexJournalWarning("failed to record required full index recovery", err))
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

	if len(warnings) > 0 || !autoReindexEnabled {
		return ProjectionResult{Warnings: warnings}
	}

	missingPaths := make([]string, 0, len(indexPaths))
	for _, relPath := range indexPaths {
		if paths.HasMDExtension(relPath) {
			missingPaths = append(missingPaths, relPath)
		}
	}
	missingRefs, err := checksvc.DetectMissingRefs(rt, missingPaths...)
	if err != nil {
		return ProjectionResult{Warnings: warnings}
	}
	sort.Slice(missingRefs, func(i, j int) bool {
		return missingRefs[i].TargetPath < missingRefs[j].TargetPath
	})
	return ProjectionResult{MissingRefs: missingRefs, Warnings: warnings}
}

// ProjectFileLocked projects one file while the caller holds ProjectionLock.
// It records dirty-journal recovery when projection fails.
func ProjectFileLocked(rt *vaultruntime.Runtime, filePath string) []ProjectionWarning {
	warning, projectionErr := projectIndexFileLocked(rt, filePath)
	if projectionErr == nil {
		return nil
	}
	warnings := []ProjectionWarning{warning}
	if err := recordProjectionRecovery(rt.VaultPath, filePath, projectionErr); err != nil {
		warnings = append(warnings, indexJournalWarning("failed to record pending index recovery", err))
	}
	return warnings
}

func uniqueProjectionPaths(groups ...[]string) []string {
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

// currentProjectionPlan rechecks removed paths while projection is serialized.
// A newer concurrent mutation may have recreated a deleted or moved path after
// this ChangeSet was produced; project that file instead of deleting its row.
func currentProjectionPlan(rt *vaultruntime.Runtime, changes mutation.ChangeSet) (removedPaths, indexPaths []string) {
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

func removeIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (ProjectionWarning, bool) {
	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to open index database", err), true
	}
	if err := rt.DB.RemoveFile(relPath); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to remove file from index", err), true
	}
	return ProjectionWarning{}, false
}

func projectIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (ProjectionWarning, error) {
	filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
	if paths.HasMDExtension(relPath) {
		return projectIndexFileLocked(rt, filePath)
	}
	return ProjectionWarning{}, nil
}

func projectIndexFileLocked(rt *vaultruntime.Runtime, filePath string) (ProjectionWarning, error) {
	vaultPath := rt.VaultPath
	if rt.SchemaLoadErr != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to load schema", rt.SchemaLoadErr), rt.SchemaLoadErr
	}
	if rt.Schema == nil {
		err := errors.New("schema runtime is required")
		return indexUpdateWarning(vaultPath, filePath, "failed to load schema", err), err
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to read file", err), err
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, rt.ParseOptions)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to parse file", err), err
	}

	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}
	if err := rt.DB.IndexDocumentWithMtime(doc, rt.Schema, mtime); err != nil {
		var resolutionErr *index.PostCommitReferenceResolutionError
		if errors.As(err, &resolutionErr) {
			return referenceResolutionIncompleteWarning(vaultPath, filePath, resolutionErr), err
		}
		return indexUpdateWarning(vaultPath, filePath, "failed to update index", err), err
	}
	return ProjectionWarning{}, nil
}

func indexUpdateWarning(vaultPath, filePath, prefix string, err error) ProjectionWarning {
	displayPath := projectionDisplayPath(vaultPath, filePath)
	return ProjectionWarning{
		Code:    codes.WarnIndexUpdateFailed,
		Message: fmt.Sprintf("auto-reindex failed for %s: %s: %v", displayPath, prefix, err),
		Ref:     IndexUpdateFailedWarningRef,
		Err:     err,
	}
}

func referenceResolutionIncompleteWarning(vaultPath, filePath string, err *index.PostCommitReferenceResolutionError) ProjectionWarning {
	displayPath := projectionDisplayPath(vaultPath, filePath)
	scope := "file"
	if err.VaultWide {
		scope = "vault-wide"
	}
	return ProjectionWarning{
		Code:    codes.WarnRefResolutionIncomplete,
		Message: fmt.Sprintf("auto-reindex indexed %s, but %s reference resolution did not complete: %v", displayPath, scope, err.Err),
		Ref:     "The file was indexed successfully, but backlinks may be stale. Run 'rvn reindex' to retry reference resolution.",
		Err:     err,
	}
}

func projectionDisplayPath(vaultPath, filePath string) string {
	displayPath := filepath.ToSlash(filepath.Clean(filePath))
	if relPath, err := filepath.Rel(vaultPath, filePath); err == nil && !strings.HasPrefix(relPath, "..") {
		displayPath = filepath.ToSlash(relPath)
	}
	return displayPath
}

func indexJournalWarning(message string, err error) ProjectionWarning {
	return ProjectionWarning{
		Code:    codes.WarnIndexUpdateFailed,
		Message: fmt.Sprintf("%s: %v", message, err),
		Ref:     IndexUpdateFailedWarningRef,
		Err:     err,
	}
}

func recordProjectionRecovery(vaultPath, filePath string, projectionErr error) error {
	var resolutionErr *index.PostCommitReferenceResolutionError
	if errors.As(projectionErr, &resolutionErr) && resolutionErr.VaultWide {
		_, err := indexjournal.RequireFullScan(vaultPath, "")
		return err
	}

	fullPath := filePath
	if !filepath.IsAbs(fullPath) {
		fullPath = filepath.Join(vaultPath, filepath.FromSlash(fullPath))
	}
	relPath, err := filepath.Rel(vaultPath, fullPath)
	if err != nil {
		return fmt.Errorf("resolve index recovery path: %w", err)
	}
	relPath = filepath.ToSlash(filepath.Clean(relPath))
	if !paths.IsValidVaultRelPath(relPath) {
		return fmt.Errorf("invalid index recovery path %q", relPath)
	}
	_, err = indexjournal.SetPaths(vaultPath, "", []string{relPath})
	return err
}
