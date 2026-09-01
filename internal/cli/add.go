package cli

import (
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
)

var addCmd = newCanonicalLeafCommand("add", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderAddResult,
})

func renderAddResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.AddResult)
	if !ok {
		if _, bulk := result.Data.(commandpayload.AddBulkPreviewResult); bulk {
			return renderCanonicalBulkResult(result)
		}
		if _, bulk := result.Data.(commandpayload.AddBulkResult); bulk {
			return renderCanonicalBulkResult(result)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	renderObjectAdded(data.File)
	renderWithWarningsAndPrompt(getVaultPath(), result)
	return nil
}

func init() {
	addCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		message := err.Error()
		if strings.Contains(message, "unknown flag: --heading") || strings.Contains(message, "unknown flag: --create-heading") {
			return handleErrorMsg(
				ErrInvalidInput,
				"rvn add only appends body content; it no longer accepts --heading or --create-heading",
				`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
			)
		}
		return err
	})
	if err := addCmd.RegisterFlagCompletionFunc("to", completeReferenceFlag(true)); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(addCmd)
}
