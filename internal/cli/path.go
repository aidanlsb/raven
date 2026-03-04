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

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the resolved vault directory path",
	Long: `Prints the configured vault directory path.

Useful for shell integration:
  cd $(rvn path)

Or add an alias to your ~/.zshrc:
  alias cdv='cd $(rvn path)'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return outputVaultPath(getVaultPath())
	},
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
		return outputVaultPath(getVaultPath())
	},
}

func init() {
	rootCmd.AddCommand(pathCmd)
}
