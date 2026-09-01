// Package cli implements the command-line interface.
package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

var updateCmd = newCanonicalLeafCommand("update", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeUpdate,
	RenderHuman: renderUpdateResult,
})


func invokeUpdate(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm, _ := cmd.Flags().GetBool("confirm")
	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		args["dry-run"] = true
	}
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
