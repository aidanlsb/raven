package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init <path>",
	Short: "Initialize a new vault",
	Long: `Creates a new vault at the specified path with default configuration files.

Creates:
  - raven.yaml   (vault configuration)
  - schema.yaml  (types and traits)
  - .raven/      (index directory)
  - .gitignore   (ignores derived files)`,
	Args: cobra.ExactArgs(1),
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

		// Create vault-level .gitignore (if it doesn't already exist)
		gitignorePath := filepath.Join(path, ".gitignore")
		createdGitignore := false
		if _, err := os.Stat(gitignorePath); os.IsNotExist(err) {
			gitignoreContent := `# Raven (auto-generated)
# These are derived files - your markdown is the source of truth

# Index database (rebuilt with 'rvn reindex')
.raven/

# Trashed files
.trash/
`
			if err := os.WriteFile(gitignorePath, []byte(gitignoreContent), 0644); err != nil {
				return fmt.Errorf("failed to create .gitignore: %w", err)
			}
			createdGitignore = true
		}

		// Create default raven.yaml (vault config)
		createdConfig, err := config.CreateDefaultVaultConfig(path)
		if err != nil {
			return fmt.Errorf("failed to create raven.yaml: %w", err)
		}

		// Create default schema.yaml
		createdSchema, err := schema.CreateDefault(path)
		if err != nil {
			return fmt.Errorf("failed to create schema.yaml: %w", err)
		}

		// Report what was done
		if createdConfig {
			fmt.Println("✓ Created raven.yaml (vault configuration)")
		} else {
			fmt.Println("• raven.yaml already exists (kept)")
		}

		if createdSchema {
			fmt.Println("✓ Created schema.yaml (types and traits)")
		} else {
			fmt.Println("• schema.yaml already exists (kept)")
		}

		fmt.Println("✓ Created .raven/ directory (index)")

		if createdGitignore {
			fmt.Println("✓ Created .gitignore (ignores derived files)")
		} else {
			fmt.Println("• .gitignore already exists (kept)")
		}

		if createdConfig || createdSchema {
			fmt.Println("\nVault initialized! Start adding markdown files.")
		} else {
			fmt.Println("\nExisting vault detected. Configuration preserved.")
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(initCmd)
}
