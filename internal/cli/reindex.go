package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var reindexCmd = newCanonicalLeafCommand("reindex", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	Prepare:      prepareReindexArgs,
	BuildArgs:    buildReindexArgs,
	Invoke:       invokeReindex,
	HandleResult: handleReindexResult,
})

func prepareReindexArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	fullReindex, _ := cmd.Flags().GetBool("full")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	if !jsonOutput && !dryRun {
		if fullReindex {
			fmt.Printf("Full reindexing vault: %s\n", ui.FilePath(getVaultPath()))
		} else {
			fmt.Printf("Reindexing vault: %s\n", ui.FilePath(getVaultPath()))
		}
	}
	return args, false, nil
}

func buildReindexArgs(cmd *cobra.Command, _ []string) (map[string]interface{}, error) {
	fullReindex, _ := cmd.Flags().GetBool("full")
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	return map[string]interface{}{
		"full":    fullReindex,
		"dry-run": dryRun,
	}, nil
}

func invokeReindex(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	return app.CommandInvoker().Execute(cmd.Context(), commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
	})
}

func handleReindexResult(cmd *cobra.Command, result commandexec.Result) error {
	if jsonOutput {
		outputJSON(result)
		return nil
	}

	dryRun, _ := cmd.Flags().GetBool("dry-run")
	data := canonicalDataMap(result)
	for _, warning := range result.Warnings {
		fmt.Fprintf(os.Stderr, "%s\n", ui.Warning(warning.Message))
	}

	if dryRun {
		incrementalResult, _ := data["incremental"].(bool)
		filesIndexed := intFromMap(data, "files_indexed")
		filesDeleted := intFromMap(data, "files_deleted")
		filesSkipped := intFromMap(data, "files_skipped")
		if incrementalResult {
			fmt.Printf("\n%s\n", ui.Starf("Dry run: %d files would be reindexed, %d deleted, %d up-to-date",
				filesIndexed, filesDeleted, filesSkipped))
		} else {
			fmt.Printf("\n%s\n", ui.Starf("Dry run: %d files would be reindexed", filesIndexed))
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
}

func init() {
	rootCmd.AddCommand(reindexCmd)
}
