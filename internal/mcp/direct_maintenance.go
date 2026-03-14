package mcp

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
)

func (s *Server) callDirectReindex(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	fullReindex := boolValue(normalized["full"])
	dryRun := boolValue(normalized["dry-run"])
	incremental := !fullReindex

	db, wasRebuilt, err := index.OpenWithRebuild(vaultPath)
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to open database: %v", err), "Run 'rvn reindex' to rebuild the database", nil), true
	}
	defer db.Close()

	if wasRebuilt {
		incremental = false
	}

	if !incremental && !dryRun {
		if err := db.ClearAllData(); err != nil {
			return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to clear database for full reindex: %v", err), "", nil), true
		}
	}

	dailyDir := vaultCfg.GetDailyDirectory()
	if dailyDir == "" {
		dailyDir = "daily"
	}
	db.SetDailyDirectory(dailyDir)

	parseOpts := parseOptionsFromVaultConfig(vaultCfg)

	_, _ = db.RemoveFilesWithPrefix(".trash/")

	var fileCount, skippedCount, errorCount, deletedCount int
	var errors []string
	var staleFiles []string
	var deletedFiles []string

	if incremental {
		if dryRun {
			indexedPaths, indexedErr := db.AllIndexedFilePaths()
			if indexedErr == nil {
				for _, relPath := range indexedPaths {
					fullPath := vaultPath + "/" + relPath
					if _, statErr := os.Stat(fullPath); os.IsNotExist(statErr) {
						deletedFiles = append(deletedFiles, relPath)
					}
				}
				deletedCount = len(deletedFiles)
			}
		} else {
			deletedFiles, _ = db.RemoveDeletedFiles(vaultPath)
			deletedCount = len(deletedFiles)
		}
	}

	walkOpts := &vault.WalkOptions{ParseOptions: parseOpts}
	walkErr := vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(result vault.WalkResult) error {
		if result.Error != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", result.RelativePath, result.Error))
			errorCount++
		} else {
			if incremental {
				indexedMtime, mtimeErr := db.GetFileMtime(result.RelativePath)
				if mtimeErr == nil && indexedMtime > 0 && result.FileMtime <= indexedMtime {
					skippedCount++
					return nil
				}
				staleFiles = append(staleFiles, result.RelativePath)
			}

			if dryRun {
				fileCount++
				return nil
			}

			if idxErr := db.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); idxErr != nil {
				errors = append(errors, fmt.Sprintf("%s: %v", result.RelativePath, idxErr))
				errorCount++
				return nil
			}

			fileCount++
		}
		return nil
	})
	if walkErr != nil {
		return errorEnvelope("FILE_READ_ERROR", fmt.Sprintf("error walking vault: %v", walkErr), "", nil), true
	}

	var refResult *index.ReferenceResolutionResult
	if !dryRun && fileCount > 0 {
		refResult, _ = db.ResolveReferencesWithSchema(dailyDir, sch)
		_ = db.Analyze()
	}

	stats, err := db.Stats()
	if err != nil {
		return errorEnvelope("DATABASE_ERROR", fmt.Sprintf("failed to get stats: %v", err), "", nil), true
	}

	data := map[string]interface{}{
		"files_indexed":  fileCount,
		"files_skipped":  skippedCount,
		"files_deleted":  deletedCount,
		"objects":        stats.ObjectCount,
		"traits":         stats.TraitCount,
		"references":     stats.RefCount,
		"schema_rebuilt": wasRebuilt,
		"incremental":    incremental,
		"dry_run":        dryRun,
		"errors":         errors,
	}
	if incremental {
		data["stale_files"] = staleFiles
		data["deleted_files"] = deletedFiles
	}
	if refResult != nil {
		data["refs_resolved"] = refResult.Resolved
		data["refs_unresolved"] = refResult.Unresolved
	}

	return successEnvelope(data, nil), false
}

func (s *Server) callDirectCheck(args map[string]interface{}) (string, bool) {
	vaultPath, vaultCfg, sch, normalized, errOut, isErr := s.directContext(args)
	if isErr {
		return errOut, true
	}

	result, err := checksvc.Run(vaultPath, vaultCfg, sch, checksvc.Options{
		PathArg:     strings.TrimSpace(toString(normalized["path"])),
		TypeFilter:  strings.TrimSpace(toString(normalized["type"])),
		TraitFilter: strings.TrimSpace(toString(normalized["trait"])),
		Issues:      strings.TrimSpace(toString(normalized["issues"])),
		Exclude:     strings.TrimSpace(toString(normalized["exclude"])),
		ErrorsOnly:  boolValue(normalized["errors-only"]),
	})
	if err != nil {
		return errorEnvelope("VALIDATION_FAILED", err.Error(), "", nil), true
	}

	if boolValue(normalized["create-missing"]) &&
		boolValue(normalized["confirm"]) &&
		result.Scope.Type == "full" {
		checksvc.CreateMissingRefsNonInteractive(
			vaultPath,
			sch,
			result.MissingRefs,
			vaultCfg.GetObjectsRoot(),
			vaultCfg.GetPagesRoot(),
			vaultCfg.GetTemplateDirectory(),
		)
	}

	jsonResult := checksvc.BuildJSON(vaultPath, result)
	encoded, err := json.Marshal(jsonResult)
	if err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to build check response", "", nil), true
	}
	var data map[string]interface{}
	if err := json.Unmarshal(encoded, &data); err != nil {
		return errorEnvelope("INTERNAL_ERROR", "failed to build check response", "", nil), true
	}
	return successEnvelope(data, nil), false
}
