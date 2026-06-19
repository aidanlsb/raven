package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
)

var backlinksCmd = newCanonicalLeafCommand("backlinks", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           cobra.MaximumNArgs(1),
	Prepare:        prepareBacklinksArgs,
	BuildArgs:      buildBacklinksArgs,
	HandleErrorCmd: handleBacklinksFailure,
	RenderHuman:    renderBacklinks,
})

func prepareBacklinksArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err != nil {
		return nil, handled, err
	}
	return prepareInteractiveReferenceArgs(args, "backlinks", "target", "backlinks> ", "Select a target for backlinks (Esc to cancel)", interactiveReferencePickerOptions{IncludeAssets: true})
}

func buildBacklinksArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"target": args[0],
	}, nil
}

func handleBacklinksFailure(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "backlinks",
		ArgKey:    "target",
		Prompt:    "backlinks/ref> ",
		Render:    renderBacklinks,
	})
}

func renderBacklinks(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	target, _ := data["target"].(string)
	links, _ := data["items"].([]model.Reference)
	browse, _ := cmd.Flags().GetBool("browse")
	if browse {
		if len(links) == 0 {
			printBacklinksResults(target, links)
			return nil
		}
		return browseAndOpenReferences("Backlinks to "+target, browseItemsForBacklinkResults(links))
	}
	printBacklinksResults(target, links)
	return nil
}

func init() {
	backlinksCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(backlinksCmd)
}

func browseItemsForBacklinkResults(links []model.Reference) []picker.Item {
	return browseItemsForReferenceResults(links, func(link model.Reference) string {
		if link.DisplayText != nil {
			return *link.DisplayText
		}
		return link.SourceID
	})
}
