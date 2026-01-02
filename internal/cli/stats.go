package cli

import (
	"fmt"
	"time"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

// StatsJSON is the JSON representation of vault statistics.
type StatsJSON struct {
	FileCount   int `json:"file_count"`
	ObjectCount int `json:"object_count"`
	TraitCount  int `json:"trait_count"`
	RefCount    int `json:"ref_count"`
	TagCount    int `json:"tag_count,omitempty"`
}

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
			outputSuccess(StatsJSON{
				FileCount:   stats.FileCount,
				ObjectCount: stats.ObjectCount,
				TraitCount:  stats.TraitCount,
				RefCount:    stats.RefCount,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable output
		fmt.Println("Vault Statistics")
		fmt.Println("================")
		fmt.Printf("Files:      %d\n", stats.FileCount)
		fmt.Printf("Objects:    %d\n", stats.ObjectCount)
		fmt.Printf("Traits:     %d\n", stats.TraitCount)
		fmt.Printf("References: %d\n", stats.RefCount)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(statsCmd)
}
