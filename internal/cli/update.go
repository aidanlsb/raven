// Package cli implements the command-line interface.
package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

var updateCmd = newCanonicalLeafCommand("update", canonicalLeafOptions{
	VaultPath: getVaultPath,
	RenderHuman: func(_ *cobra.Command, result commandexec.Result) error {
		return renderCanonicalBulkResult(result)
	},
})

func init() {
	rootCmd.AddCommand(updateCmd)
}
