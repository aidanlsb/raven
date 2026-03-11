package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/readsvc"
)

var backlinksCmd = &cobra.Command{
	Use:   "backlinks <target>",
	Short: "Show backlinks to an object",
	Long: `Shows all references pointing to the specified object.

The target can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

Examples:
  rvn backlinks freya                    # Resolves to people/freya
  rvn backlinks people/freya
  rvn backlinks daily/2025-02-01#standup
  rvn backlinks people/freya --json`,
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
			VaultPath:    rt.VaultPath,
			VaultConfig:  rt.VaultCfg,
			AllowMissing: true,
		}, true)
		if err != nil {
			return handleResolveError(err, reference)
		}
		target := result.ObjectID

		links, err := readsvc.Backlinks(rt, target)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		readsvc.SaveBacklinksResults(vaultPath, target, links)

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			outputSuccess(map[string]interface{}{
				"target": target,
				"items":  links,
			}, &Meta{Count: len(links), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		printBacklinksResults(target, links)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
