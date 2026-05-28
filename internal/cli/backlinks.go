package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var backlinksCmd = newCanonicalLeafCommand("backlinks", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MaximumNArgs(1),
	Prepare:     prepareBacklinksArgs,
	BuildArgs:   buildBacklinksArgs,
	RenderHuman: renderBacklinks,
})

func prepareBacklinksArgs(_ *cobra.Command, args []string) ([]string, bool, error) {
	return prepareInteractiveReferenceArgs(args, "backlinks", "target", "backlinks> ", "Select a target for backlinks (Esc to cancel)")
}

func buildBacklinksArgs(_ *cobra.Command, args []string) (map[string]interface{}, error) {
	return map[string]interface{}{
		"target": args[0],
	}, nil
}

func renderBacklinks(_ *cobra.Command, result commandexec.Result) error {
	data := canonicalDataMap(result)
	target, _ := data["target"].(string)
	links, _ := data["items"].([]model.Reference)
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
