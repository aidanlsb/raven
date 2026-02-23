package cli

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/docsync"
	"github.com/aidanlsb/raven/internal/schema"
)

type initDocsResult struct {
	Fetched   bool   `json:"fetched"`
	FileCount int    `json:"file_count,omitempty"`
	StorePath string `json:"store_path,omitempty"`
}

type initResult struct {
	Path           string         `json:"path"`
	Status         string         `json:"status"`
	CreatedConfig  bool           `json:"created_config"`
	CreatedSchema  bool           `json:"created_schema"`
	GitignoreState string         `json:"gitignore_state"`
	Docs           initDocsResult `json:"docs"`
}

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

		if !isJSONOutput() {
			fmt.Printf("Initializing vault at: %s\n", path)
		}

		// Create directory if it doesn't exist
		if err := os.MkdirAll(path, 0755); err != nil {
			return handleError(ErrFileWriteError, fmt.Errorf("failed to create vault directory: %w", err), "Check that the destination path is writable")
		}

		// Create .raven directory
		ravenDir := filepath.Join(path, ".raven")
		if err := os.MkdirAll(ravenDir, 0755); err != nil {
			return handleError(ErrFileWriteError, fmt.Errorf("failed to create .raven directory: %w", err), "Check that the destination path is writable")
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
				return handleError(ErrFileWriteError, fmt.Errorf("failed to write .gitignore: %w", err), "Check write permissions for .gitignore")
			}
		} else if existingContent != "" {
			gitignoreStatus = "unchanged"
		}

		// Create default raven.yaml (vault config)
		createdConfig, err := config.CreateDefaultVaultConfig(path)
		if err != nil {
			return handleError(ErrFileWriteError, fmt.Errorf("failed to create raven.yaml: %w", err), "")
		}

		// Create default schema.yaml
		createdSchema, err := schema.CreateDefault(path)
		if err != nil {
			return handleError(ErrFileWriteError, fmt.Errorf("failed to create schema.yaml: %w", err), "")
		}

		var (
			docsResult initDocsResult
			warnings   []Warning
		)
		info := currentVersionInfo()
		fetchResult, fetchErr := docsync.Fetch(docsync.FetchOptions{
			VaultPath:  path,
			CLIVersion: info.Version,
			HTTPClient: &http.Client{Timeout: 60 * time.Second},
		})

		if fetchErr != nil {
			warnings = append(warnings, Warning{
				Code:    WarnDocsFetchFailed,
				Message: fmt.Sprintf("Docs fetch failed: %v. Run 'rvn --vault-path %s docs fetch' to retry.", fetchErr, path),
			})
		} else {
			docsResult = initDocsResult{
				Fetched:   true,
				FileCount: fetchResult.FileCount,
				StorePath: docsync.StoreRelPath,
			}
		}

		status := "existing"
		if createdConfig || createdSchema {
			status = "initialized"
		}

		if isJSONOutput() {
			result := initResult{
				Path:           path,
				Status:         status,
				CreatedConfig:  createdConfig,
				CreatedSchema:  createdSchema,
				GitignoreState: gitignoreStatus,
				Docs:           docsResult,
			}
			if len(warnings) > 0 {
				outputSuccessWithWarnings(result, warnings, nil)
			} else {
				outputSuccess(result, nil)
			}
			return nil
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

		if fetchErr != nil {
			fmt.Printf("! Docs fetch failed: %v\n", fetchErr)
			fmt.Printf("  Run 'rvn --vault-path %s docs fetch' to retry.\n", path)
		} else {
			fmt.Printf("✓ Fetched docs into %s (%d files)\n", docsync.StoreRelPath, fetchResult.FileCount)
		}

		if status == "initialized" {
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
