package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func outputKeepPath(path string) error {
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
	Short: "Print the resolved keep directory path",
	Long: `Prints the configured keep directory path.

Useful for shell integration:
  cd $(rvn path)

Or add an alias to your ~/.zshrc:
  alias cdv='cd $(rvn path)'`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return outputKeepPath(getKeepPath())
	},
}

var keepPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the resolved keep directory path",
	Long: `Print the resolved keep directory path.

Useful for shell integration:
  cd $(rvn keep path)

This command resolves the keep the same way regular keep-bound commands do:
explicit --keep-path, then --keep, then active/default config.`,
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		return outputKeepPath(getKeepPath())
	},
}

func init() {
	rootCmd.AddCommand(pathCmd)
}
