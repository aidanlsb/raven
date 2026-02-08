package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		start := time.Now()

		// Load vault config
		vaultCfg := loadVaultConfigSafe(vaultPath)

		// Resolve the reference to get the canonical object ID
		// Use dynamic date resolution so "today", "yesterday", etc. work.
		result, err := resolveReferenceWithDynamicDates(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		}, true)
		if err != nil {
			return handleResolveError(err, reference)
		}
		source := result.ObjectID

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		links, err := db.Outlinks(source)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		saveLastOutlinksResults(vaultPath, source, links)

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

func saveLastOutlinksResults(vaultPath, source string, links []model.Reference) {
	modelResults := make([]model.Result, len(links))
	for i, link := range links {
		modelResults[i] = link
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceOutlinks, "", source, modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func init() {
	rootCmd.AddCommand(outlinksCmd)
}
