package cli

import (
	"github.com/spf13/cobra"
)

func canonicalGroupDefaultRunE(commandID string, render humanRenderer) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		result := executeCanonicalCommand(commandID, canonicalVaultPath(commandID, nil), nil)
		return finishCanonicalLeaf(cmd, result, render, nil)
	}
}
