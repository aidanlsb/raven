package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var (
	readRawFlag     bool
	readNoLinksFlag bool
)

var readCmd = &cobra.Command{
	Use:   "read <reference>",
	Short: "Read a file with context",
	Long: `Read and output a file from the vault.

The reference can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

By default, this command shows enriched output (rendered wikilinks and backlinks).
Use --raw to output only the raw file content (useful for agents and scripting).

Examples:
  rvn read freya                  # Resolves to people/freya.md
  rvn read people/freya
  rvn read daily/2025-02-01.md
  rvn read people/freya --raw
  rvn read people/freya --raw --json`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]
		start := time.Now()

		// Apply hyperlink preference for this run.
		setHyperlinksDisabled(readNoLinksFlag)

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

		if readRawFlag {
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
		}

		return readEnriched(readEnrichedOptions{
			vaultPath:   vaultPath,
			vaultCfg:    vaultCfg,
			reference:   reference,
			objectID:    result.ObjectID,
			fileAbsPath: fullPath,
			fileRelPath: relPath,
			content:     string(content),
			lineCount:   lineCount,
			start:       start,
			elapsedMs:   elapsed,
		})
	},
}

func init() {
	readCmd.Flags().BoolVar(&readRawFlag, "raw", false, "Output only raw file content (no backlinks, no rendered links)")
	readCmd.Flags().BoolVar(&readNoLinksFlag, "no-links", false, "Disable clickable hyperlinks in terminal output")
	rootCmd.AddCommand(readCmd)
}
