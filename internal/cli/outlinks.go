package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var outlinksCmd = &cobra.Command{
	Use:   "outlinks <source>",
	Short: "Show outlinks from an object",
	Long: `Shows all references made by the specified object.

The source can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

Examples:
  rvn outlinks freya                    # Resolves to people/freya
  rvn outlinks people/freya
  rvn outlinks daily/2025-02-01#standup
  rvn outlinks people/freya --json`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "outlinks",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"source": reference,
			},
		})
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

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data, _ := result.Data.(map[string]interface{})
		source, _ := data["source"].(string)
		links, _ := data["items"].([]model.Reference)
		printOutlinksResults(source, links)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(outlinksCmd)
}
