package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var readCmd = &cobra.Command{
	Use:   "read <reference>",
	Short: "Read raw file content",
	Long: `Read and output the raw content of a file in the vault.

The reference can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

This command is useful for agents that need full file context for 
summarization or understanding structure.

Examples:
  rvn read freya                  # Resolves to people/freya.md
  rvn read people/freya
  rvn read daily/2025-02-01.md
  rvn read people/freya --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		start := time.Now()

		// Load vault config
		vaultCfg := loadVaultConfigSafe(vaultPath)

		// Resolve the reference using unified resolver
		result, err := ResolveReference(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			return handleResolveError(err, reference)
		}

		fullPath := result.FilePath
		relPath, _ := filepath.Rel(vaultPath, fullPath)

		// Read file
		content, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("file not found: %s", relPath), "Check the path and try again")
			}
			return handleError(ErrFileReadError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		// Count lines
		lineCount := strings.Count(string(content), "\n")
		if len(content) > 0 && content[len(content)-1] != '\n' {
			lineCount++ // Account for last line without newline
		}

		if isJSONOutput() {
			outputSuccess(FileResult{
				Path:      relPath,
				Content:   string(content),
				LineCount: lineCount,
			}, &Meta{QueryTimeMs: elapsed})
			return nil
		}

		// Human-readable: just output the content
		fmt.Print(string(content))
		return nil
	},
}

func init() {
	rootCmd.AddCommand(readCmd)
}
