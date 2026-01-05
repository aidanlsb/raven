package cli

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Reindex all files",
	Long: `Parses all markdown files in the vault and rebuilds the SQLite index.

By default, reindexes all files. Use --smart for incremental mode that only
reindexes files that have changed since the last index.

Examples:
  # Full reindex
  rvn reindex

  # Smart incremental reindex (only changed files)
  rvn reindex --smart

  # Check what would be reindexed without doing it
  rvn reindex --smart --dry-run`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		smart, _ := cmd.Flags().GetBool("smart")
		dryRun, _ := cmd.Flags().GetBool("dry-run")

		if !jsonOutput && !dryRun {
			if smart {
				fmt.Printf("Smart reindexing vault: %s\n", vaultPath)
			} else {
				fmt.Printf("Reindexing vault: %s\n", vaultPath)
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
			smart = false
			if !jsonOutput {
				fmt.Println("Database schema was outdated - performing full reindex.")
			}
		}

		// Clean up files in excluded directories (.trash/, etc.)
		// These should never be in the index but might exist from before this check was added
		trashRemoved, err := db.RemoveFilesWithPrefix(".trash/")
		if err != nil && !jsonOutput {
			fmt.Fprintf(os.Stderr, "Warning: failed to clean up trash files from index: %v\n", err)
		}
		if trashRemoved > 0 && !jsonOutput && !dryRun {
			fmt.Printf("Cleaned up %d files from .trash/ in index\n", trashRemoved)
		}

		var fileCount, skippedCount, errorCount int
		var errors []string
		var staleFiles []string

		// Walk all markdown files
		err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
			if result.Error != nil {
				if !jsonOutput {
					fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", result.RelativePath, result.Error)
				}
				errors = append(errors, fmt.Sprintf("%s: %v", result.RelativePath, result.Error))
				errorCount++
				return nil
			}

			// In smart mode, check if file needs reindexing
			if smart {
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

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		// Resolve references after all files are indexed
		var refResult *index.ReferenceResolutionResult
		if !dryRun && fileCount > 0 {
			dailyDir := "daily" // default
			if vaultCfg, err := config.LoadVaultConfig(vaultPath); err == nil && vaultCfg.DailyDirectory != "" {
				dailyDir = vaultCfg.DailyDirectory
			}

			refResult, err = db.ResolveReferences(dailyDir)
			if err != nil && !jsonOutput {
				fmt.Fprintf(os.Stderr, "Warning: failed to resolve references: %v\n", err)
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
				"objects":        stats.ObjectCount,
				"traits":         stats.TraitCount,
				"references":     stats.RefCount,
				"schema_rebuilt": wasRebuilt,
				"smart_mode":     smart,
				"dry_run":        dryRun,
				"errors":         errors,
			}
			if smart {
				data["stale_files"] = staleFiles
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
			if smart {
				fmt.Printf("\nDry run: %d files would be reindexed, %d up-to-date\n", fileCount, skippedCount)
			} else {
				fmt.Printf("\nDry run: %d files would be reindexed\n", fileCount)
			}
		} else {
			fmt.Println()
			if smart && skippedCount > 0 {
				fmt.Printf("✓ Indexed %d changed files (%d up-to-date)\n", fileCount, skippedCount)
			} else {
				fmt.Printf("✓ Indexed %d files\n", fileCount)
			}
			fmt.Printf("  %d objects\n", stats.ObjectCount)
			fmt.Printf("  %d traits\n", stats.TraitCount)
			if refResult != nil && refResult.Unresolved > 0 {
				fmt.Printf("  %d references (%d resolved, %d unresolved)\n",
					stats.RefCount, refResult.Resolved, refResult.Unresolved)
			} else {
				fmt.Printf("  %d references\n", stats.RefCount)
			}

			if errorCount > 0 {
				fmt.Printf("  %d errors\n", errorCount)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reindexCmd)
	reindexCmd.Flags().Bool("smart", false, "Only reindex files that have changed since last index")
	reindexCmd.Flags().Bool("dry-run", false, "Show what would be reindexed without doing it")
}
