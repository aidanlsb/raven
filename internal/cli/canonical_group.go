package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func canonicalGroupDefaultRunE(commandID string, vaultPath func() string, render func(*cobra.Command, commandexec.Result) error) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		path := ""
		if vaultPath != nil {
			path = vaultPath()
		}

		result := executeCanonicalCommand(commandID, path, nil)
		if isJSONOutput() {
			outputCanonicalResultJSON(result)
			return nil
		}
		if err := handleCanonicalFailure(result); err != nil {
			return err
		}
		return render(cmd, result)
	}
}
