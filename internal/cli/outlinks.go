package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/readsvc"
)

var outlinksCmd = newExceptionLeafCommand("outlinks", exceptionLeafOptions{
	Args:        validateOutlinksArgs,
	Prepare:     prepareOutlinksArgs,
	HandleError: handleOutlinksFailure,
	RenderHuman: renderOutlinks,
})

func validateOutlinksArgs(cmd *cobra.Command, args []string) error {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin && len(args) > 0 {
		return fmt.Errorf("cannot specify reference when using --stdin")
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
	return prepareInteractiveReferenceArgs(args, "outlinks", "reference", "outlinks> ", "Select a reference for outlinks (Esc to cancel)")
}

func handleOutlinksFailure(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "outlinks",
		ArgKey:    "reference",
		Prompt:    "outlinks/ref> ",
		Render:    renderOutlinks,
	})
}

func renderOutlinks(cmd *cobra.Command, result commandexec.Result) error {
	if grouped, ok := result.Data.(readsvc.OutlinksStdinPayload); ok {
		browse, _ := cmd.Flags().GetBool("browse")
		if browse {
			if outlinksGroupItemCount(grouped.ItemsBySource) == 0 {
				printOutlinksGroups(grouped.ItemsBySource, grouped.Errors)
				return nil
			}
			return browseOutlinkGroups(grouped.ItemsBySource)
		}
		printOutlinksGroups(grouped.ItemsBySource, grouped.Errors)
		return nil
	}
	data, err := commandResultData[readsvc.OutlinksPayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	browse, _ := cmd.Flags().GetBool("browse")
	if browse {
		if len(data.Items) == 0 {
			printOutlinksResults(data.Source, data.Items)
			return nil
		}
		return browseAndOpenReferences("Outlinks from "+data.Source, browseItemsForOutlinkResults(data.Items))
	}
	printOutlinksResults(data.Source, data.Items)
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
