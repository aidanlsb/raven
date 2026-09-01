package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
)

var backlinksCmd = newCanonicalLeafCommand("backlinks", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           validateBacklinksArgs,
	Prepare:        prepareBacklinksArgs,
	HandleErrorCmd: handleBacklinksFailure,
	RenderHuman:    renderBacklinks,
})

func validateBacklinksArgs(cmd *cobra.Command, args []string) error {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin && len(args) > 0 {
		return fmt.Errorf("cannot specify reference when using --stdin")
	}
	if len(args) > 1 {
		return fmt.Errorf("accepts at most 1 argument")
	}
	return nil
}

func prepareBacklinksArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err != nil {
		return nil, handled, err
	}
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		return args, false, nil
	}
	return prepareInteractiveReferenceArgs(args, "backlinks", "reference", "backlinks> ", "Select a reference for backlinks (Esc to cancel)")
}


func handleBacklinksFailure(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "backlinks",
		ArgKey:    "reference",
		Prompt:    "backlinks/ref> ",
		Render:    renderBacklinks,
	})
}

func renderBacklinks(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if groups, ok := data["items_by_target"].([]model.BacklinksGroup); ok {
		errors := referenceInputErrorsFromAny(data["errors"])
		browse, _ := cmd.Flags().GetBool("browse")
		if browse {
			if backlinksGroupItemCount(groups) == 0 {
				printBacklinksGroups(groups, errors)
				return nil
			}
			return browseBacklinkGroups(groups)
		}
		printBacklinksGroups(groups, errors)
		return nil
	}
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

func backlinksGroupItemCount(groups []model.BacklinksGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Items)
	}
	return total
}

func init() {
	backlinksCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    true,
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
