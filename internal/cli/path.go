package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the vault directory path",
	Long: `Prints the configured vault directory path.

Useful for shell integration:
  cd $(rvn path)

Or add an alias to your ~/.zshrc:
  alias cdv='cd $(rvn path)'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		path := getVaultPath()
		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"path": path,
			}, nil)
			return nil
		}
		fmt.Println(path)
		return nil
	},
}

func init() {
	rootCmd.AddCommand(pathCmd)
}
