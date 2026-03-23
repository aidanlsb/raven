package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
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
		incremental := !fullReindex

		if !jsonOutput && !dryRun {
			if incremental {
				fmt.Printf("Reindexing vault: %s\n", ui.FilePath(vaultPath))
			} else {
				fmt.Printf("Full reindexing vault: %s\n", ui.FilePath(vaultPath))
			}
		}

		result := app.CommandInvoker().Execute(ctx, commandexec.Request{
			CommandID: "reindex",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"full":    fullReindex,
				"dry-run": dryRun,
			},
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if jsonOutput {
			outputJSON(result)
			return nil
		}

		data, _ := result.Data.(map[string]interface{})
		for _, warning := range result.Warnings {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning.Message)
		}

		if dryRun {
			incrementalResult, _ := data["incremental"].(bool)
			filesIndexed := intFromMap(data, "files_indexed")
			filesDeleted := intFromMap(data, "files_deleted")
			filesSkipped := intFromMap(data, "files_skipped")
			if incrementalResult {
				fmt.Printf("\nDry run: %d files would be reindexed, %d deleted, %d up-to-date\n",
					filesIndexed, filesDeleted, filesSkipped)
			} else {
				fmt.Printf("\nDry run: %d files would be reindexed\n", filesIndexed)
			}
			return nil
		}

		filesIndexed := intFromMap(data, "files_indexed")
		filesDeleted := intFromMap(data, "files_deleted")
		filesSkipped := intFromMap(data, "files_skipped")
		incrementalResult, _ := data["incremental"].(bool)
		fmt.Println()
		if incrementalResult && (filesSkipped > 0 || filesDeleted > 0) {
			if filesDeleted > 0 {
				fmt.Println(ui.Checkf("Indexed %d changed files, removed %d deleted %s",
					filesIndexed, filesDeleted, ui.Hint(fmt.Sprintf("(%d up-to-date)", filesSkipped))))
			} else {
				fmt.Println(ui.Checkf("Indexed %d changed files %s", filesIndexed, ui.Hint(fmt.Sprintf("(%d up-to-date)", filesSkipped))))
			}
		} else {
			fmt.Println(ui.Checkf("Indexed %d files", filesIndexed))
		}
		fmt.Printf("  %s objects\n", ui.Bold.Render(fmt.Sprintf("%d", intFromMap(data, "objects"))))
		fmt.Printf("  %s traits\n", ui.Bold.Render(fmt.Sprintf("%d", intFromMap(data, "traits"))))
		if intFromMap(data, "refs_unresolved") > 0 {
			fmt.Printf("  %s references %s\n",
				ui.Bold.Render(fmt.Sprintf("%d", intFromMap(data, "references"))),
				ui.Hint(fmt.Sprintf("(%d resolved, %d unresolved)", intFromMap(data, "refs_resolved"), intFromMap(data, "refs_unresolved"))))
		} else {
			fmt.Printf("  %s references\n", ui.Bold.Render(fmt.Sprintf("%d", intFromMap(data, "references"))))
		}

		if errorsRaw, ok := data["errors"].([]interface{}); ok && len(errorsRaw) > 0 {
			fmt.Printf("  %s\n", ui.Errorf("%d errors", len(errorsRaw)))
		} else if errorsRaw, ok := data["errors"].([]string); ok && len(errorsRaw) > 0 {
			fmt.Printf("  %s\n", ui.Errorf("%d errors", len(errorsRaw)))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reindexCmd)
	reindexCmd.Flags().Bool("full", false, "Force full reindex of all files (default is incremental)")
	reindexCmd.Flags().Bool("dry-run", false, "Show what would be reindexed without doing it")
}
