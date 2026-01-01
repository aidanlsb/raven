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
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(getVaultPath())
	},
}

func init() {
	rootCmd.AddCommand(pathCmd)
}
