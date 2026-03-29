package cli

import (
	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/model"
)

var outlinksCmd = newCanonicalLeafCommand("outlinks", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	RenderHuman: renderOutlinks,
})

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
