package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/reindexsvc"
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

		result, err := reindexsvc.Run(reindexsvc.RunRequest{
			VaultPath: vaultPath,
			Full:      fullReindex,
			DryRun:    dryRun,
			Context:   ctx,
		})
		if err != nil {
			return mapReindexServiceError(err)
		}

		if jsonOutput {
			outputSuccess(result.Data(), nil)
			return nil
		}

		for _, warning := range result.WarningMessages {
			fmt.Fprintf(os.Stderr, "Warning: %s\n", warning)
		}

		if dryRun {
			if result.Incremental {
				fmt.Printf("\nDry run: %d files would be reindexed, %d deleted, %d up-to-date\n",
					result.FilesIndexed, result.FilesDeleted, result.FilesSkipped)
			} else {
				fmt.Printf("\nDry run: %d files would be reindexed\n", result.FilesIndexed)
			}
			return nil
		}

		fmt.Println()
		if result.Incremental && (result.FilesSkipped > 0 || result.FilesDeleted > 0) {
			if result.FilesDeleted > 0 {
				fmt.Println(ui.Checkf("Indexed %d changed files, removed %d deleted %s",
					result.FilesIndexed, result.FilesDeleted, ui.Hint(fmt.Sprintf("(%d up-to-date)", result.FilesSkipped))))
			} else {
				fmt.Println(ui.Checkf("Indexed %d changed files %s", result.FilesIndexed, ui.Hint(fmt.Sprintf("(%d up-to-date)", result.FilesSkipped))))
			}
		} else {
			fmt.Println(ui.Checkf("Indexed %d files", result.FilesIndexed))
		}
		fmt.Printf("  %s objects\n", ui.Bold.Render(fmt.Sprintf("%d", result.Objects)))
		fmt.Printf("  %s traits\n", ui.Bold.Render(fmt.Sprintf("%d", result.Traits)))
		if result.HasRefResult && result.RefsUnresolved > 0 {
			fmt.Printf("  %s references %s\n",
				ui.Bold.Render(fmt.Sprintf("%d", result.References)),
				ui.Hint(fmt.Sprintf("(%d resolved, %d unresolved)", result.RefsResolved, result.RefsUnresolved)))
		} else {
			fmt.Printf("  %s references\n", ui.Bold.Render(fmt.Sprintf("%d", result.References)))
		}

		if len(result.Errors) > 0 {
			fmt.Printf("  %s\n", ui.Errorf("%d errors", len(result.Errors)))
		}

		return nil
	},
}

func mapReindexServiceError(err error) error {
	svcErr, ok := reindexsvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	suggestion := svcErr.Suggestion
	svcCause := svcErr.Unwrap()

	switch svcErr.Code {
	case reindexsvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, suggestion)
	case reindexsvc.CodeSchemaInvalid:
		if svcCause != nil {
			return handleError(ErrSchemaInvalid, svcCause, suggestion)
		}
		return handleErrorMsg(ErrSchemaInvalid, svcErr.Message, suggestion)
	case reindexsvc.CodeConfigInvalid:
		if svcCause != nil {
			return handleError(ErrConfigInvalid, svcCause, suggestion)
		}
		return handleErrorMsg(ErrConfigInvalid, svcErr.Message, suggestion)
	case reindexsvc.CodeDatabaseError:
		if svcCause != nil {
			return handleError(ErrDatabaseError, svcCause, suggestion)
		}
		return handleErrorMsg(ErrDatabaseError, svcErr.Message, suggestion)
	case reindexsvc.CodeFileReadError:
		if svcCause != nil {
			return handleError(ErrFileReadError, svcCause, suggestion)
		}
		return handleErrorMsg(ErrFileReadError, svcErr.Message, suggestion)
	default:
		if svcCause != nil {
			return handleError(ErrInternal, svcCause, suggestion)
		}
		return handleErrorMsg(ErrInternal, svcErr.Message, suggestion)
	}
}

func init() {
	rootCmd.AddCommand(reindexCmd)
	reindexCmd.Flags().Bool("full", false, "Force full reindex of all files (default is incremental)")
	reindexCmd.Flags().Bool("dry-run", false, "Show what would be reindexed without doing it")
}
