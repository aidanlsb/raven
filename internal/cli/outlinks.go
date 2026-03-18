package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/readsvc"
)

var outlinksCmd = &cobra.Command{
	Use:   "outlinks <source>",
	Short: "Show outlinks from an object",
	Long: `Shows all references made by the specified object.

The source can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

Examples:
  rvn outlinks freya                    # Resolves to people/freya
  rvn outlinks people/freya
  rvn outlinks daily/2025-02-01#standup
  rvn outlinks people/freya --json`,
	Args: cobra.ExactArgs(1),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		start := time.Now()

		rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: true})
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		defer rt.Close()

		// Resolve the reference to get the canonical object ID
		// Use dynamic date resolution so "today", "yesterday", etc. work.
		result, err := resolveReferenceWithDynamicDates(reference, ResolveOptions{
			VaultPath:   rt.VaultPath,
			VaultConfig: rt.VaultCfg,
		}, true)
		if err != nil {
			return handleResolveError(err, reference)
		}
		source := result.ObjectID

		links, err := readsvc.Outlinks(rt, source)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"source": source,
				"items":  links,
			}, &Meta{Count: len(links), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		printOutlinksResults(source, links)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(outlinksCmd)
}
