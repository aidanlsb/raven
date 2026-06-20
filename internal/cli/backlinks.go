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
	BuildArgs:      buildBacklinksArgs,
	HandleErrorCmd: handleBacklinksFailure,
	RenderHuman:    renderBacklinks,
})

func validateBacklinksArgs(cmd *cobra.Command, args []string) error {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin && len(args) > 0 {
		return fmt.Errorf("cannot specify target when using --stdin")
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
	return prepareInteractiveReferenceArgs(args, "backlinks", "target", "backlinks> ", "Select a target for backlinks (Esc to cancel)", interactiveReferencePickerOptions{IncludeAssets: true})
}

func buildBacklinksArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		targets, err := ReadReferencesFromStdin()
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			return nil, fmt.Errorf("no targets provided on stdin")
		}
		return map[string]interface{}{
			"stdin":   true,
			"targets": targets,
		}, nil
	}
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
