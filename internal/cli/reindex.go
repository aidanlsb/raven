package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Reindex all files",
	Long: `Parses all markdown files in the vault and rebuilds the SQLite index.

By default, performs an incremental reindex that only processes files that have
changed since the last index. Deleted files are automatically detected and
removed from the index.

Use --full to force a complete rebuild of the entire index.

Examples:
  # Incremental reindex (default - only changed/deleted files)
  rvn reindex

  # Full reindex (rebuild everything)
  rvn reindex --full

  # Check what would be reindexed without doing it
  rvn reindex --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		ctx := cmd.Context()
		fullReindex, _ := cmd.Flags().GetBool("full")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		// Incremental is the default, --full forces complete rebuild
		incremental := !fullReindex

		if !jsonOutput && !dryRun {
			if incremental {
				fmt.Printf("Reindexing vault: %s\n", ui.FilePath(vaultPath))
			} else {
				fmt.Printf("Full reindexing vault: %s\n", ui.FilePath(vaultPath))
			}
		}

		// Load schema
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		// Open database (rebuild if schema incompatible)
		db, wasRebuilt, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		// If schema was rebuilt, force full reindex
		if wasRebuilt {
			incremental = false
			if !jsonOutput {
				fmt.Println(ui.Info("Database schema was outdated - performing full reindex."))
			}
		}

		// For full reindex, clear all existing data first to avoid ID conflicts
		// (e.g., after directory migration where file paths change but IDs stay the same)
		if !incremental && !dryRun {
			if err := db.ClearAllData(); err != nil {
				return fmt.Errorf("failed to clear database for full reindex: %w", err)
			}
		}

		// Load vault config for directory roots
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
		dailyDir := vaultCfg.GetDailyDirectory()
		if dailyDir == "" {
			dailyDir = "daily"
		}

		// Build parse options from vault config
		parseOpts := buildParseOptions(vaultCfg)

		// Clean up files in excluded directories (.trash/, etc.)
		// These should never be in the index but might exist from before this check was added
		trashRemoved, err := db.RemoveFilesWithPrefix(".trash/")
		if err != nil && !jsonOutput {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean up trash files from index: %v\n", err)
		}
		if trashRemoved > 0 && !jsonOutput && !dryRun {
			fmt.Println(ui.Infof("Cleaned up %d files from .trash/ in index", trashRemoved))
		}

		var fileCount, skippedCount, errorCount, deletedCount int
		var errors []string
		var staleFiles []string
		var deletedFiles []string

		// In incremental mode, first detect and remove deleted files
		if incremental {
			if dryRun {
				// In dry-run mode, just detect deleted files without removing
				indexedPaths, err := db.AllIndexedFilePaths()
				if err != nil && !jsonOutput {
					fmt.Fprintf(os.Stderr, "Warning: failed to check for deleted files: %v\n", err)
				} else {
					for _, relPath := range indexedPaths {
						fullPath := vaultPath + "/" + relPath
						if _, err := os.Stat(fullPath); os.IsNotExist(err) {
							deletedFiles = append(deletedFiles, relPath)
							if !jsonOutput {
								fmt.Printf("  Would remove (deleted): %s\n", relPath)
							}
						}
					}
					deletedCount = len(deletedFiles)
				}
			} else {
				// Actually remove deleted files
				deletedFiles, err = db.RemoveDeletedFiles(vaultPath)
				if err != nil && !jsonOutput {
					fmt.Fprintf(os.Stderr, "Warning: failed to clean up deleted files: %v\n", err)
				}
				deletedCount = len(deletedFiles)
				if deletedCount > 0 && !jsonOutput {
					fmt.Println(ui.Infof("Removed %d deleted files from index", deletedCount))
				}
			}
		}

		// Walk all markdown files with parse options
		var spinner *ui.Spinner
		if !jsonOutput && !dryRun {
			spinner = ui.NewSpinner("Indexing files")
			spinner.Start()
		}

		walkOpts := &vault.WalkOptions{ParseOptions: parseOpts}
		err = vault.WalkMarkdownFilesWithOptions(vaultPath, walkOpts, func(result vault.WalkResult) error {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}

			if result.Error != nil {
				if !jsonOutput {
					fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", result.RelativePath, result.Error)
				}
				errors = append(errors, fmt.Sprintf("%s: %v", result.RelativePath, result.Error))
				errorCount++
				return nil
			}

			// In incremental mode, check if file needs reindexing
			if incremental {
				indexedMtime, err := db.GetFileMtime(result.RelativePath)
				if err == nil && indexedMtime > 0 && result.FileMtime <= indexedMtime {
					// File hasn't changed since last index
					skippedCount++
					return nil
				}
				staleFiles = append(staleFiles, result.RelativePath)
			}

			// Dry run mode - just report, don't index
			if dryRun {
				if !jsonOutput {
					fmt.Printf("  Would reindex: %s\n", result.RelativePath)
				}
				fileCount++
				return nil
			}

			// Index document with mtime
			if err := db.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); err != nil {
				if !jsonOutput {
					fmt.Fprintf(os.Stderr, "Error indexing %s: %v\n", result.RelativePath, err)
				}
				errors = append(errors, fmt.Sprintf("%s: %v", result.RelativePath, err))
				errorCount++
				return nil
			}

			fileCount++
			return nil
		})

		if spinner != nil {
			spinner.Stop()
		}

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		// Resolve references after all files are indexed
		var refResult *index.ReferenceResolutionResult
		if !dryRun && fileCount > 0 {
			refResult, err = db.ResolveReferences(dailyDir)
			if err != nil && !jsonOutput {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve references: %v\n", err)
			}

			// Update query planner statistics for optimal performance
			if err := db.Analyze(); err != nil && !jsonOutput {
				fmt.Fprintf(os.Stderr, "Warning: failed to analyze database: %v\n", err)
			}
		}

		stats, err := db.Stats()
		if err != nil {
			return fmt.Errorf("failed to get stats: %w", err)
		}

		if jsonOutput {
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
			result := map[string]interface{}{
				"ok":   true,
				"data": data,
			}
			out, _ := json.MarshalIndent(result, "", "  ")
			fmt.Println(string(out))
		} else if dryRun {
			if incremental {
				fmt.Printf("\nDry run: %d files would be reindexed, %d deleted, %d up-to-date\n",
					fileCount, deletedCount, skippedCount)
			} else {
				fmt.Printf("\nDry run: %d files would be reindexed\n", fileCount)
			}
		} else {
			fmt.Println()
			if incremental && (skippedCount > 0 || deletedCount > 0) {
				if deletedCount > 0 {
					fmt.Println(ui.Checkf("Indexed %d changed files, removed %d deleted %s",
						fileCount, deletedCount, ui.Hint(fmt.Sprintf("(%d up-to-date)", skippedCount))))
				} else {
					fmt.Println(ui.Checkf("Indexed %d changed files %s", fileCount, ui.Hint(fmt.Sprintf("(%d up-to-date)", skippedCount))))
				}
			} else {
				fmt.Println(ui.Checkf("Indexed %d files", fileCount))
			}
			fmt.Printf("  %s objects\n", ui.Bold.Render(fmt.Sprintf("%d", stats.ObjectCount)))
			fmt.Printf("  %s traits\n", ui.Bold.Render(fmt.Sprintf("%d", stats.TraitCount)))
			if refResult != nil && refResult.Unresolved > 0 {
				fmt.Printf("  %s references %s\n",
					ui.Bold.Render(fmt.Sprintf("%d", stats.RefCount)),
					ui.Hint(fmt.Sprintf("(%d resolved, %d unresolved)", refResult.Resolved, refResult.Unresolved)))
			} else {
				fmt.Printf("  %s references\n", ui.Bold.Render(fmt.Sprintf("%d", stats.RefCount)))
			}

			if errorCount > 0 {
				fmt.Printf("  %s\n", ui.Errorf("%d errors", errorCount))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reindexCmd)
	reindexCmd.Flags().Bool("full", false, "Force full reindex of all files (default is incremental)")
	reindexCmd.Flags().Bool("dry-run", false, "Show what would be reindexed without doing it")
}
