package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ravenscroftj/raven/internal/index"
	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
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

		// Open database
		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		var fileCount, errorCount int

		// Get canonical vault path for security check
		canonicalVault, err := filepath.Abs(vaultPath)
		if err != nil {
			canonicalVault = vaultPath
		}

		// Walk all markdown files
		err = filepath.WalkDir(vaultPath, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil // Skip errors
			}

			// Skip directories
			if d.IsDir() {
				// Skip .raven directory
				if d.Name() == ".raven" {
					return filepath.SkipDir
				}
				return nil
			}

			// Only process .md files
			if !strings.HasSuffix(path, ".md") {
				return nil
			}

			// Security: verify file is within vault
			canonicalFile, err := filepath.Abs(path)
			if err != nil {
				return nil
			}
			if !strings.HasPrefix(canonicalFile, canonicalVault) {
				return nil // Skip files outside vault
			}

			// Read file
			content, err := os.ReadFile(path)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error reading %s: %v\n", path, err)
				errorCount++
				return nil
			}

			// Parse document
			doc, err := parser.ParseDocument(string(content), path, vaultPath)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error parsing %s: %v\n", path, err)
				errorCount++
				return nil
			}

			// Index document
			if err := db.IndexDocument(doc, sch); err != nil {
				fmt.Fprintf(os.Stderr, "Error indexing %s: %v\n", path, err)
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
