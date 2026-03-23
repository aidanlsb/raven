package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var backlinksCmd = &cobra.Command{
	Use:   "backlinks <target>",
	Short: "Show backlinks to an object",
	Long: `Shows all references pointing to the specified object.

The target can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

Examples:
  rvn backlinks freya                    # Resolves to people/freya
  rvn backlinks people/freya
  rvn backlinks daily/2025-02-01#standup
  rvn backlinks people/freya --json`,
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
			CommandID: "backlinks",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"target": reference,
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
		target, _ := data["target"].(string)
		links, _ := data["items"].([]model.Reference)
		printBacklinksResults(target, links)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
