package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/schema"
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

		// Ensure .gitignore has Raven entries
		gitignorePath := filepath.Join(path, ".gitignore")
		gitignoreStatus := "created"
		ravenGitignoreEntries := []string{".raven/", ".trash/"}

		existingContent := ""
		if data, err := os.ReadFile(gitignorePath); err == nil {
			existingContent = string(data)
		}

		// Check which entries are missing
		var missingEntries []string
		for _, entry := range ravenGitignoreEntries {
			if !strings.Contains(existingContent, entry) {
				missingEntries = append(missingEntries, entry)
			}
		}

		if len(missingEntries) > 0 {
			var newContent string
			if existingContent == "" {
				// Create new file
				newContent = `# Raven (auto-generated)
# These are derived files - your markdown is the source of truth

# Index database (rebuilt with 'rvn reindex')
.raven/

# Trashed files
.trash/
`
			} else {
				// Append to existing file
				gitignoreStatus = "updated"
				addition := "\n# Raven\n"
				for _, entry := range missingEntries {
					addition += entry + "\n"
				}
				newContent = strings.TrimRight(existingContent, "\n") + "\n" + addition
			}
			if err := os.WriteFile(gitignorePath, []byte(newContent), 0644); err != nil {
				return fmt.Errorf("failed to write .gitignore: %w", err)
			}
		} else if existingContent != "" {
			gitignoreStatus = "already has Raven entries"
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

		fmt.Println("✓ Ensured .raven/ directory exists")

		switch gitignoreStatus {
		case "created":
			fmt.Println("✓ Created .gitignore")
		case "updated":
			fmt.Println("✓ Updated .gitignore (added Raven entries)")
		default:
			fmt.Println("• .gitignore already has Raven entries")
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
