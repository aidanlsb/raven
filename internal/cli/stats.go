package cli

import (
	"fmt"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var statsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show index statistics",
	Long:  `Displays statistics about the vault index.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		stats, err := db.Stats()
		if err != nil {
			return fmt.Errorf("failed to get stats: %w", err)
		}

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
