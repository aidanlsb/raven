package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
)

var outlinksCmd = newCanonicalLeafCommand("outlinks", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           validateOutlinksArgs,
	Prepare:        prepareOutlinksArgs,
	BuildArgs:      buildOutlinksArgs,
	HandleErrorCmd: handleOutlinksFailure,
	RenderHuman:    renderOutlinks,
})

func validateOutlinksArgs(cmd *cobra.Command, args []string) error {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin && len(args) > 0 {
		return fmt.Errorf("cannot specify source when using --stdin")
	}
	if len(args) > 1 {
		return fmt.Errorf("accepts at most 1 argument")
	}
	return nil
}

func prepareOutlinksArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err != nil {
		return nil, handled, err
	}
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		return args, false, nil
	}
	return prepareInteractiveReferenceArgs(args, "outlinks", "source", "outlinks> ", "Select a source for outlinks (Esc to cancel)", interactiveReferencePickerOptions{})
}

func buildOutlinksArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		sources, err := ReadReferencesFromStdin()
		if err != nil {
			return nil, err
		}
		if len(sources) == 0 {
			return nil, fmt.Errorf("no sources provided on stdin")
		}
		return map[string]interface{}{
			"stdin":   true,
			"sources": sources,
		}, nil
	}
	return map[string]interface{}{
		"source": args[0],
	}, nil
}

func handleOutlinksFailure(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "outlinks",
		ArgKey:    "source",
		Prompt:    "outlinks/ref> ",
		Render:    renderOutlinks,
	})
}

func renderOutlinks(cmd *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	if groups, ok := data["items_by_source"].([]model.OutlinksGroup); ok {
		errors := referenceInputErrorsFromAny(data["errors"])
		browse, _ := cmd.Flags().GetBool("browse")
		if browse {
			if outlinksGroupItemCount(groups) == 0 {
				printOutlinksGroups(groups, errors)
				return nil
			}
			return browseOutlinkGroups(groups)
		}
		printOutlinksGroups(groups, errors)
		return nil
	}
	source, _ := data["source"].(string)
	links, _ := data["items"].([]model.Reference)
	browse, _ := cmd.Flags().GetBool("browse")
	if browse {
		if len(links) == 0 {
			printOutlinksResults(source, links)
			return nil
		}
		return browseAndOpenReferences("Outlinks from "+source, browseItemsForOutlinkResults(links))
	}
	printOutlinksResults(source, links)
	return nil
}

func outlinksGroupItemCount(groups []model.OutlinksGroup) int {
	total := 0
	for _, group := range groups {
		total += len(group.Items)
	}
	return total
}

func init() {
	outlinksCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(outlinksCmd)
}

func browseItemsForOutlinkResults(links []model.Reference) []picker.Item {
	return browseItemsForReferenceResults(links, func(link model.Reference) string {
		target := link.TargetRaw
		if link.DisplayText != nil && *link.DisplayText != "" && *link.DisplayText != link.TargetRaw {
			target = *link.DisplayText + " (" + link.TargetRaw + ")"
		}
		return target
	})
}
