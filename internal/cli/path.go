package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func outputVaultPath(path string) error {
	if isJSONOutput() {
		outputSuccess(map[string]interface{}{
			"path": path,
		}, nil)
		return nil
	}
	fmt.Println(path)
	return nil
}

var vaultPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the resolved vault directory path",
	Long: `Print the resolved vault directory path.

Useful for shell integration:
  cd $(rvn vault path)

This command resolves the vault the same way regular vault-bound commands do:
explicit --vault-path, then --vault, then active/default config.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		result := executeCanonicalCommand("vault_path", getVaultPath(), nil)
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

		data := canonicalDataMap(result)
		return outputVaultPath(stringValue(data["path"]))
	},
}
