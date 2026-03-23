// Package cli implements the command-line interface.
package cli

import (
	"context"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
)

var (
	updateStdin   bool
	updateConfirm bool
)

var updateCmd = &cobra.Command{
	Use:   "update <trait_id> <new_value>",
	Short: "Update a trait's value",
	Long: `Update the value of a trait annotation.

Trait IDs look like "path/file.md:trait:N" and can be obtained via:
  - rvn query "trait:todo" --ids

Bulk operations:
  Use --stdin to read trait IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn update daily/2026-01-25.md:trait:0 done
  rvn query "trait:todo" --ids | rvn update --stdin done --confirm`,
	Args: cobra.ArbitraryArgs,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()

	if updateStdin {
		newValue, err := parseTraitUpdateValueArgs(args, "Usage: rvn update --stdin <new_value>")
		if err != nil {
			return err
		}

		ids, err := ReadTraitIDsFromStdin()
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no trait IDs provided via stdin", "Pipe trait IDs to stdin, one per line")
		}

		return executeCanonicalUpdate(vaultPath, map[string]interface{}{
			"stdin":      true,
			"value":      newValue,
			"object_ids": stringsToAny(ids),
		})
	}

	if len(args) < 2 {
		return handleErrorMsg(ErrMissingArgument, "requires trait-id and new value arguments", "Usage: rvn update <trait_id> <new_value>")
	}

	traitID := args[0]
	if !strings.Contains(traitID, ":trait:") {
		return handleErrorMsg(ErrInvalidInput, "invalid trait ID format", "Trait IDs look like: path/file.md:trait:N")
	}

	newValue, err := parseTraitUpdateValueArgs(args[1:], "Usage: rvn update <trait_id> <new_value>")
	if err != nil {
		return err
	}

	return executeCanonicalUpdate(vaultPath, map[string]interface{}{
		"trait_id": traitID,
		"value":    newValue,
	})
}

func executeCanonicalUpdate(vaultPath string, args map[string]interface{}) error {
	result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
		CommandID: "update",
		VaultPath: vaultPath,
		Caller:    commandexec.CallerCLI,
		Args:      args,
		Confirm:   updateConfirm,
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

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	return renderCanonicalBulkResult(result)
}

func init() {
	updateCmd.Flags().BoolVar(&updateStdin, "stdin", false, "Read trait IDs from stdin (one per line)")
	updateCmd.Flags().BoolVar(&updateConfirm, "confirm", false, "Apply bulk changes (without this flag, shows preview only)")
	rootCmd.AddCommand(updateCmd)
}
