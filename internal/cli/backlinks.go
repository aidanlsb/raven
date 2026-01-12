package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/ui"
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
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			vaultCfg = &config.VaultConfig{}
		}

		// Resolve the reference to get the canonical object ID
		result, err := ResolveReference(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
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

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			items := make([]BacklinkResult, len(links))
			for i, link := range links {
				items[i] = BacklinkResult{
					SourceID:    link.SourceID,
					FilePath:    link.FilePath,
					Line:        link.Line,
					DisplayText: link.DisplayText,
				}
			}
			outputSuccess(map[string]interface{}{
				"target": target,
				"items":  items,
			}, &Meta{Count: len(items), QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		if len(links) == 0 {
			fmt.Println(ui.Starf("No backlinks found for '%s'", target))
			return nil
		}

		fmt.Printf("%s %s\n\n", ui.Header("Backlinks to "+target), ui.Hint(fmt.Sprintf("(%d)", len(links))))
		for _, link := range links {
			display := link.SourceID
			if link.DisplayText != nil {
				display = *link.DisplayText
			}

			line := 0
			if link.Line != nil {
				line = *link.Line
			}

			fmt.Printf("  %s %s %s\n", ui.SymbolAttention, ui.Accent.Render(display), ui.Muted.Render(formatLocationLinkSimple(link.FilePath, line)))
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
