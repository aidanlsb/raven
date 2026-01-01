package cli

import (
	"fmt"
	"os"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/ravenscroftj/raven/internal/vault"
	"github.com/spf13/cobra"
)

var reindexCmd = &cobra.Command{
	Use:   "reindex",
	Short: "Reindex all files",
	Long:  `Parses all markdown files in the vault and rebuilds the SQLite index.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		fmt.Printf("Reindexing vault: %s\n", vaultPath)

		// Load schema
		sch, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		// Open database (rebuild if schema incompatible)
		db, wasRebuilt, err := index.OpenWithRebuild(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		if wasRebuilt {
			fmt.Println("Database schema was outdated - rebuilt from scratch.")
		}

		var fileCount, errorCount int

		// Walk all markdown files
		err = vault.WalkMarkdownFiles(vaultPath, func(result vault.WalkResult) error {
			if result.Error != nil {
				fmt.Fprintf(os.Stderr, "Error processing %s: %v\n", result.RelativePath, result.Error)
				errorCount++
				return nil
			}

			// Index document
			if err := db.IndexDocument(result.Document, sch); err != nil {
				fmt.Fprintf(os.Stderr, "Error indexing %s: %v\n", result.RelativePath, err)
				errorCount++
				return nil
			}

			fileCount++
			return nil
		})

		if err != nil {
			return fmt.Errorf("error walking vault: %w", err)
		}

		stats, err := db.Stats()
		if err != nil {
			return fmt.Errorf("failed to get stats: %w", err)
		}

		fmt.Println()
		fmt.Printf("✓ Indexed %d files\n", fileCount)
		fmt.Printf("  %d objects\n", stats.ObjectCount)
		fmt.Printf("  %d traits\n", stats.TraitCount)
		fmt.Printf("  %d references\n", stats.RefCount)

		if errorCount > 0 {
			fmt.Printf("  %d errors\n", errorCount)
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(reindexCmd)
}
