package cli

import (
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
)

var outlinksCmd = newCanonicalLeafCommand("outlinks", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           cobra.MaximumNArgs(1),
	Prepare:        prepareOutlinksArgs,
	BuildArgs:      buildOutlinksArgs,
	HandleErrorCmd: handleOutlinksFailure,
	RenderHuman:    renderOutlinks,
})

func prepareOutlinksArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	if handled, err := validateReferenceBrowseFlag(cmd); handled || err != nil {
		return nil, handled, err
	}
	return prepareInteractiveReferenceArgs(args, "outlinks", "source", "outlinks> ", "Select a source for outlinks (Esc to cancel)")
}

func buildOutlinksArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
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
	source, _ := data["source"].(string)
	links, _ := data["items"].([]model.Reference)
	browse, _ := cmd.Flags().GetBool("browse")
	if browse {
		if len(links) == 0 {
			printOutlinksResults(source, links)
			return nil
		}
		item, ok, err := browseReferences("Outlinks from "+source, browseItemsForOutlinkResults(links))
		if err != nil || !ok {
			return err
		}
		openFileInEditorAtLine(filepath.Join(getVaultPath(), item.FilePath), item.FilePath, item.Line, false)
		return nil
	}
	printOutlinksResults(source, links)
	return nil
}

func init() {
	outlinksCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
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
