package reindexsvc

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

type Code string

const (
	CodeInvalidInput  Code = "INVALID_INPUT"
	CodeSchemaInvalid Code = "SCHEMA_INVALID"
	CodeConfigInvalid Code = "CONFIG_INVALID"
	CodeDatabaseError Code = "DATABASE_ERROR"
	CodeFileReadError Code = "FILE_READ_ERROR"
	CodeInternal      Code = "INTERNAL_ERROR"
)

type Error struct {
	Code       Code
	Message    string
	Suggestion string
	Err        error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newError(code Code, message, suggestion string, err error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Err: err}
}

func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
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
	Objects       int
	Traits        int
	References    int
	SchemaRebuilt bool
	Incremental   bool
	DryRun        bool
	Errors        []string

	StaleFiles   []string
	DeletedFiles []string

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

	db, wasRebuilt, err := index.OpenWithRebuild(vaultPath)
	if err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to open database: %v", err), "Run 'rvn reindex' to rebuild the database", err)
	}
	defer db.Close()

	incremental := !req.Full
	if wasRebuilt {
		incremental = false
	}

	if !incremental && !req.DryRun {
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

	parseOpts := buildParseOptions(vaultCfg)

	result := &RunResult{
		SchemaRebuilt:   wasRebuilt,
		Incremental:     incremental,
		DryRun:          req.DryRun,
		Errors:          []string{},
		StaleFiles:      []string{},
		DeletedFiles:    []string{},
		WarningMessages: []string{},
		HasRefResult:    false,
		RefsResolved:    0,
		RefsUnresolved:  0,
		FilesIndexed:    0,
		FilesSkipped:    0,
		FilesDeleted:    0,
		Objects:         0,
		Traits:          0,
		References:      0,
	}
	dryRunFileStats := make(map[string]index.IndexStats)
	dryRunStats := index.IndexStats{}

	trashRemoved, err := db.RemoveFilesWithPrefix(".trash/")
	if err != nil {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to clean up trash files from index: %v", err))
	}
	if trashRemoved > 0 {
		result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("Cleaned up %d files from .trash/ in index", trashRemoved))
	}

	if incremental {
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

	walkOpts := &vault.WalkOptions{ParseOptions: parseOpts}
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

		result.FilesIndexed++
		return nil
	})
	if walkErr != nil {
		return nil, newError(CodeFileReadError, fmt.Sprintf("error walking vault: %v", walkErr), "", walkErr)
	}

	if req.DryRun {
		if incremental {
			projected, err := projectedDryRunStats(db, result.DeletedFiles, dryRunFileStats)
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

	if !req.DryRun && result.FilesIndexed > 0 {
		refResult, refErr := db.ResolveReferencesWithSchema(dailyDir, sch)
		if refErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to resolve references: %v", refErr))
		} else if refResult != nil {
			result.RefsResolved = refResult.Resolved
			result.RefsUnresolved = refResult.Unresolved
			result.HasRefResult = true
		}

		if analyzeErr := db.Analyze(); analyzeErr != nil {
			result.WarningMessages = append(result.WarningMessages, fmt.Sprintf("failed to analyze database: %v", analyzeErr))
		}
	}

	stats, err := db.Stats()
	if err != nil {
		return nil, newError(CodeDatabaseError, fmt.Sprintf("failed to get stats: %v", err), "", err)
	}
	result.Objects = stats.ObjectCount
	result.Traits = stats.TraitCount
	result.References = stats.RefCount

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

func buildParseOptions(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil || !vaultCfg.HasDirectoriesConfig() {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}
