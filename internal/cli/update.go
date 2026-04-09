// Package cli implements the command-line interface.
package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

var updateCmd = newCanonicalLeafCommand("update", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.ArbitraryArgs,
	BuildArgs:   buildUpdateArgs,
	Invoke:      invokeUpdate,
	RenderHuman: renderUpdateResult,
})

func buildUpdateArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		newValue, err := parseTraitUpdateValueArgs(args, "Usage: rvn update --stdin <new_value>")
		if err != nil {
			return nil, err
		}

		ids, err := ReadTraitIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		if len(ids) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no trait IDs provided via stdin", "Pipe trait IDs to stdin, one per line")
		}

		return map[string]interface{}{
			"stdin":     true,
			"value":     newValue,
			"trait_ids": stringsToAny(ids),
		}, nil
	}

	if len(args) < 2 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires trait-id and new value arguments", "Usage: rvn update <trait_id> <new_value>")
	}

	traitID := args[0]
	if !strings.Contains(traitID, ":trait:") {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid trait ID format", "Trait IDs look like: path/file.md:trait:N")
	}

	newValue, err := parseTraitUpdateValueArgs(args[1:], "Usage: rvn update <trait_id> <new_value>")
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"trait_id": traitID,
		"value":    newValue,
	}, nil
}

func invokeUpdate(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   confirm,
	})
}

func renderUpdateResult(_ *cobra.Command, result commandexec.Result) error {
	return renderCanonicalBulkResult(result)
}

func init() {
	rootCmd.AddCommand(updateCmd)
}
