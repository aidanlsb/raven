package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize a new vault",
	Long:  `Creates a new vault at the specified path with a default schema.yaml.`,
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := args[0]

		fmt.Printf("Initializing vault at: %s\n", path)

		// Create directory if it doesn't exist
		if err := os.MkdirAll(path, 0755); err != nil {
			return fmt.Errorf("failed to create vault directory: %w", err)
		}

		// Create .raven directory
		ravenDir := filepath.Join(path, ".raven")
		if err := os.MkdirAll(ravenDir, 0755); err != nil {
			return fmt.Errorf("failed to create .raven directory: %w", err)
		}

		// Create .gitignore for .raven directory
		gitignorePath := filepath.Join(path, ".raven", ".gitignore")
		if err := os.WriteFile(gitignorePath, []byte("index.db\nindex.db-*\n"), 0644); err != nil {
			return fmt.Errorf("failed to create .gitignore: %w", err)
		}

		// Create default schema.yaml
		if err := schema.CreateDefault(path); err != nil {
			return fmt.Errorf("failed to create schema.yaml: %w", err)
		}

		fmt.Println("✓ Created schema.yaml")
		fmt.Println("✓ Created .raven/ directory")
		fmt.Println("\nVault initialized! Start adding markdown files.")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
