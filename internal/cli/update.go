// Package cli implements the command-line interface.
package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var (
	updateStdin   bool
	updateConfirm bool
)

var updateCmd = &cobra.Command{
	Use:   "update <trait_id> value=<new_value>",
	Short: "Update a trait's value",
	Long: `Update the value of a trait annotation.

Trait IDs look like "path/file.md:trait:N" and can be obtained via:
  - rvn query "trait:todo" --ids
  - rvn last <nums>

Bulk operations:
  Use --stdin to read trait IDs from stdin (one per line).
  Bulk operations preview changes by default; use --confirm to apply.

Examples:
  rvn update daily/2026-01-25.md:trait:0 value=done
  rvn query "trait:todo" --ids | rvn update --stdin value=done --confirm`,
	Args: cobra.ArbitraryArgs,
	RunE: runUpdate,
}

func runUpdate(cmd *cobra.Command, args []string) error {
	vaultPath := getVaultPath()
	vaultCfg := loadVaultConfigSafe(vaultPath)

	if updateStdin {
		if len(args) == 0 {
			return handleErrorMsg(ErrMissingArgument, "no value specified", "Usage: rvn update --stdin value=<new_value>")
		}
		newValue, err := parseTraitValueArgs(args)
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
		return handleErrorMsg(ErrMissingArgument, "requires trait-id and value=<new_value> arguments", "Usage: rvn update <trait_id> value=<new_value>")
	}

	traitID := args[0]
	if !strings.Contains(traitID, ":trait:") {
		return handleErrorMsg(ErrInvalidInput, "invalid trait ID format", "Trait IDs look like: path/file.md:trait:N")
	}

	newValue, err := parseTraitValueArgs(args[1:])
	if err != nil {
		return err
	}

	// Single update applies immediately (no preview/confirm)
	return applyUpdateTraitsByID(vaultPath, []string{traitID}, newValue, true, false, vaultCfg)
}

func parseTraitValueArgs(args []string) (string, error) {
	var newValue string
	for _, arg := range args {
		parts := strings.SplitN(arg, "=", 2)
		if len(parts) != 2 {
			return "", handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("invalid field format: %s", arg),
				"Use format: value=<new_value>")
		}
		if parts[0] != "value" {
			return "", handleErrorMsg(ErrInvalidInput,
				fmt.Sprintf("unknown field for trait: %s", parts[0]),
				"For traits, only 'value' can be updated. Example: value=done")
		}
		newValue = parts[1]
	}

	if newValue == "" {
		return "", handleErrorMsg(ErrMissingArgument, "no value specified", "Usage: value=<new_value>")
	}

	return newValue, nil
}

func init() {
	updateCmd.Flags().BoolVar(&updateStdin, "stdin", false, "Read trait IDs from stdin (one per line)")
	updateCmd.Flags().BoolVar(&updateConfirm, "confirm", false, "Apply bulk changes (without this flag, shows preview only)")
	rootCmd.AddCommand(updateCmd)
}
