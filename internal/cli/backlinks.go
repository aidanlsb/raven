package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var backlinksCmd = newCanonicalLeafCommand("backlinks", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderBacklinks,
})

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
