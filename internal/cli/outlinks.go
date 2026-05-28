package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var outlinksCmd = newCanonicalLeafCommand("outlinks", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MaximumNArgs(1),
	Prepare:     prepareOutlinksArgs,
	BuildArgs:   buildOutlinksArgs,
	RenderHuman: renderOutlinks,
})

func prepareOutlinksArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	return prepareInteractiveReferenceArgs(args, "outlinks", "source", "outlinks> ", "Select a source for outlinks (Esc to cancel)")
}

func buildOutlinksArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"source": args[0],
	}, nil
}

func renderOutlinks(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	source, _ := data["source"].(string)
	links, _ := data["items"].([]model.Reference)
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
