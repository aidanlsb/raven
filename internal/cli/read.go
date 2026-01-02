package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// FileContentJSON is the JSON representation of file content.
type FileContentJSON struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	LineCount int    `json:"line_count"`
}

var readCmd = &cobra.Command{
	Use:   "read <path>",
	Short: "Read raw file content",
	Long: `Read and output the raw content of a file in the vault.

This command is useful for agents that need full file context for 
summarization or understanding structure.

Examples:
  rvn read daily/2025-02-01.md
  rvn read people/alice.md
  rvn read daily/2025-02-01.md --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		filePath := args[0]
		start := time.Now()

		// Ensure .md extension
		if !strings.HasSuffix(filePath, ".md") {
			filePath = filePath + ".md"
		}

		// Build full path
		fullPath := filepath.Join(vaultPath, filePath)

		// Security: verify path is within vault
		absVault, err := filepath.Abs(vaultPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		absFile, err := filepath.Abs(fullPath)
		if err != nil {
			return handleError(ErrInternal, err, "")
		}
		if !strings.HasPrefix(absFile, absVault+string(filepath.Separator)) && absFile != absVault {
			return handleErrorMsg(ErrFileOutsideVault, fmt.Sprintf("path '%s' is outside vault", filePath), "")
		}

		// Read file
		content, err := os.ReadFile(fullPath)
		if err != nil {
			if os.IsNotExist(err) {
				return handleErrorMsg(ErrFileNotFound, fmt.Sprintf("file not found: %s", filePath), "Check the path and try again")
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
			outputSuccess(FileContentJSON{
				Path:      filePath,
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
