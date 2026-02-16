package cli

import (
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/lastresults"
	"github.com/aidanlsb/raven/internal/model"
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
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		start := time.Now()

		// Load vault config
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		// Resolve the reference to get the canonical object ID
		// Use dynamic date resolution so "today", "yesterday", etc. work.
		result, err := resolveReferenceWithDynamicDates(reference, ResolveOptions{
			VaultPath:    vaultPath,
			VaultConfig:  vaultCfg,
			AllowMissing: true,
		}, true)
		if err != nil {
			return handleResolveError(err, reference)
		}
		target := result.ObjectID

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		links, err := db.Backlinks(target)
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		saveLastBacklinksResults(vaultPath, target, links)

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

func saveLastBacklinksResults(vaultPath, target string, links []model.Reference) {
	modelResults := make([]model.Result, len(links))
	for i, link := range links {
		modelResults[i] = link
	}
	lr, err := lastresults.NewFromResults(lastresults.SourceBacklinks, "", target, modelResults)
	if err != nil {
		return
	}
	_ = lastresults.Write(vaultPath, lr)
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
