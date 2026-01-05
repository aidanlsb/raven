package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/index"
)

var untypedCmd = &cobra.Command{
	Use:   "untyped",
	Short: "List untyped pages",
	Long:  `Lists all files that are using the fallback 'page' type.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		pages, err := db.UntypedPages()
		if err != nil {
			return fmt.Errorf("failed to query untyped pages: %w", err)
		}

		if len(pages) == 0 {
			fmt.Println("All files have explicit types! ✓")
			return nil
		}

		fmt.Println("Untyped pages (using 'page' fallback):")
		fmt.Println()
		for _, page := range pages {
			fmt.Printf("  %s\n", page)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(untypedCmd)
}
