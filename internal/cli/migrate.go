package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/schema"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate vault to latest format",
	Long: `Migrate vault configuration and files to the latest Raven format.

This command handles:
- Schema format upgrades (schema.yaml)
- Syntax migrations (deprecated trait syntax in markdown files)
- Configuration migrations (new raven.yaml format)

Run with --dry-run first to preview changes.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		dryRun, _ := cmd.Flags().GetBool("dry-run")
		migrateSchema, _ := cmd.Flags().GetBool("schema")
		migrateSyntax, _ := cmd.Flags().GetBool("syntax")
		migrateAll, _ := cmd.Flags().GetBool("all")

		if migrateAll {
			migrateSchema = true
			migrateSyntax = true
		}

		if !migrateSchema && !migrateSyntax {
			// Default: check what needs migration
			return checkMigrationNeeds(vaultPath)
		}

		if dryRun {
			fmt.Println("=== DRY RUN - No changes will be made ===")
			fmt.Println()
		}

		var changesMade bool

		if migrateSchema {
			changed, err := runSchemaMigration(vaultPath, dryRun)
			if err != nil {
				return err
			}
			changesMade = changesMade || changed
		}

		if migrateSyntax {
			changed, err := runSyntaxMigration(vaultPath, dryRun)
			if err != nil {
				return err
			}
			changesMade = changesMade || changed
		}

		if !changesMade {
			fmt.Println("✓ Vault is up to date. No migrations needed.")
		} else if dryRun {
			fmt.Println("\nRun without --dry-run to apply changes.")
		} else {
			fmt.Println("\n✓ Migration complete.")
		}

		return nil
	},
}

func checkMigrationNeeds(vaultPath string) error {
	fmt.Printf("Checking vault: %s\n\n", vaultPath)

	result, err := schema.LoadWithWarnings(vaultPath)
	if err != nil {
		return err
	}

	needsMigration := false

	// Check schema version
	if len(result.Warnings) > 0 {
		fmt.Println("Schema:")
		for _, w := range result.Warnings {
			fmt.Printf("  ⚠ %s\n", w.Message)
		}
		needsMigration = true
	}

	// TODO: Check for deprecated syntax in files
	// This would scan files for @task(...) syntax etc.

	if !needsMigration {
		fmt.Println("✓ Vault is up to date. No migrations needed.")
	} else {
		fmt.Println("\nRun 'rvn migrate --all' to apply migrations.")
	}

	return nil
}

func runSchemaMigration(vaultPath string, dryRun bool) (bool, error) {
	schemaPath := filepath.Join(vaultPath, "schema.yaml")

	// Check if schema exists
	if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
		fmt.Println("Schema: No schema.yaml found (using defaults)")
		return false, nil
	}

	result, err := schema.LoadWithWarnings(vaultPath)
	if err != nil {
		return false, err
	}

	if result.Schema.Version >= schema.CurrentSchemaVersion {
		fmt.Printf("Schema: Already at version %d\n", result.Schema.Version)
		return false, nil
	}

	fmt.Printf("Schema: Upgrading from version %d to %d\n", result.Schema.Version, schema.CurrentSchemaVersion)

	if dryRun {
		fmt.Println("  Would update schema.yaml with version field")
		// TODO: Show more detailed migration preview
		return true, nil
	}

	// Create backup
	backupDir := filepath.Join(vaultPath, ".raven", "backups", time.Now().Format("2006-01-02T15-04-05"))
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		return false, fmt.Errorf("failed to create backup directory: %w", err)
	}

	// Copy schema.yaml to backup
	data, err := os.ReadFile(schemaPath)
	if err != nil {
		return false, err
	}
	backupPath := filepath.Join(backupDir, "schema.yaml")
	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return false, err
	}
	fmt.Printf("  Backed up to %s\n", backupPath)

	// TODO: Implement actual schema migration logic
	// For now, just add version field if missing
	// A full implementation would transform the schema structure

	fmt.Println("  ⚠ Full schema migration not yet implemented")
	fmt.Println("  Manual migration steps:")
	fmt.Println("    1. Add 'version: 2' at top of schema.yaml")
	fmt.Println("    2. Convert trait fields to single-value format")
	fmt.Println("    3. Move CLI aliases to queries in raven.yaml")

	return false, fmt.Errorf("schema migration not yet implemented; see manual steps above")
}

func runSyntaxMigration(vaultPath string, dryRun bool) (bool, error) {
	fmt.Println("Syntax: Checking for deprecated trait syntax...")

	// TODO: Implement syntax migration
	// This would:
	// 1. Scan all .md files for @task(...) @remind(...) etc.
	// 2. Parse the old format
	// 3. Convert to new atomic format
	// 4. Write back to files

	fmt.Println("  ⚠ Syntax migration not yet implemented")
	fmt.Println("  Manual migration steps:")
	fmt.Println("    Find: @task(due=DATE, priority=LEVEL)")
	fmt.Println("    Replace: @due(DATE) @priority(LEVEL)")

	return false, fmt.Errorf("syntax migration not yet implemented; see manual steps above")
}

func init() {
	migrateCmd.Flags().Bool("dry-run", false, "Preview changes without applying")
	migrateCmd.Flags().Bool("schema", false, "Migrate schema.yaml format")
	migrateCmd.Flags().Bool("syntax", false, "Migrate deprecated syntax in markdown files")
	migrateCmd.Flags().Bool("all", false, "Migrate everything")
	rootCmd.AddCommand(migrateCmd)
}
