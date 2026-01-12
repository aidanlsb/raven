package cli

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/ui"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	Long: `Displays statistics about the vault index.

Examples:
  rvn stats
  rvn stats --json`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		db, err := index.Open(vaultPath)
		if err != nil {
			return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
		}
		defer db.Close()

		stats, err := db.Stats()
		if err != nil {
			return handleError(ErrDatabaseError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		if isJSONOutput() {
			outputSuccess(StatsResult{
				FileCount:   stats.FileCount,
				ObjectCount: stats.ObjectCount,
				TraitCount:  stats.TraitCount,
				RefCount:    stats.RefCount,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		fmt.Println(ui.Header("Vault Statistics"))
		fmt.Printf("%s  %s\n", ui.Muted.Render("Files:     "), ui.Accent.Render(fmt.Sprintf("%d", stats.FileCount)))
		fmt.Printf("%s  %s\n", ui.Muted.Render("Objects:   "), ui.Accent.Render(fmt.Sprintf("%d", stats.ObjectCount)))
		fmt.Printf("%s  %s\n", ui.Muted.Render("Traits:    "), ui.Accent.Render(fmt.Sprintf("%d", stats.TraitCount)))
		fmt.Printf("%s  %s\n", ui.Muted.Render("References:"), ui.Accent.Render(fmt.Sprintf("%d", stats.RefCount)))

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
