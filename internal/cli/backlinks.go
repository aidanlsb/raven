package cli

import (
	"fmt"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/spf13/cobra"
)

var backlinksCmd = &cobra.Command{
	Use:   "backlinks <target>",
	Short: "Show backlinks to an object",
	Long: `Shows all references pointing to the specified object.

Examples:
  rvn backlinks people/alice
  rvn backlinks daily/2025-02-01#standup`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		target := args[0]

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		links, err := db.Backlinks(target)
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

		if len(links) == 0 {
			fmt.Printf("No backlinks found for '%s'\n", target)
			return nil
		}

		fmt.Printf("Backlinks to '%s':\n\n", target)
		for _, link := range links {
			display := link.SourceID
			if link.DisplayText != nil {
				display = *link.DisplayText
			}

			line := 0
			if link.Line != nil {
				line = *link.Line
			}

			fmt.Printf("  ← %s (%s:%d)\n", display, link.FilePath, line)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(backlinksCmd)
}
