// Package cli implements the command-line interface.
package cli

import (
	"strings"

	"github.com/spf13/cobra"
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
  - rvn last <nums>

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
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

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

		return applyUpdateTraitsByID(vaultPath, ids, newValue, updateConfirm, false, vaultCfg)
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

	// Single update applies immediately (no preview/confirm)
	return applyUpdateTraitsByID(vaultPath, []string{traitID}, newValue, true, false, vaultCfg)
}

func init() {
	updateCmd.Flags().BoolVar(&updateStdin, "stdin", false, "Read trait IDs from stdin (one per line)")
	updateCmd.Flags().BoolVar(&updateConfirm, "confirm", false, "Apply bulk changes (without this flag, shows preview only)")
	rootCmd.AddCommand(updateCmd)
}
