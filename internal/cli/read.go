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
	readLinesFlag   bool
	readStartLine   int
	readEndLine     int
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
  rvn read people/freya --raw --lines
  rvn read people/freya --raw --start-line 10 --end-line 40
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

		// If the caller requested line-based output/ranges, force raw mode so the content is stable.
		if readLinesFlag || readStartLine > 0 || readEndLine > 0 {
			readRawFlag = true
		}

		if readRawFlag {
			contentStr := string(content)
			startLine := 1
			endLine := lineCount
			if readStartLine > 0 {
				startLine = readStartLine
			}
			if readEndLine > 0 {
				endLine = readEndLine
			}

			if startLine < 1 || endLine < 1 || startLine > endLine || endLine > lineCount {
				return handleErrorMsg(ErrInvalidInput,
					fmt.Sprintf("invalid line range: start_line=%d end_line=%d (file has %d lines)", startLine, endLine, lineCount),
					"Use 1-indexed inclusive line numbers within the file's line_count")
			}

			// Build exact byte ranges for each line so we can return a substring without transcription risk.
			type rng struct {
				start int
				end   int // exclusive, includes trailing '\n' if present
			}
			ranges := make([]rng, 0, lineCount)
			lineStart := 0
			for i := 0; i < len(contentStr) && len(ranges) < lineCount; i++ {
				if contentStr[i] == '\n' {
					ranges = append(ranges, rng{start: lineStart, end: i + 1})
					lineStart = i + 1
				}
			}
			// If file doesn't end with '\n', record the last line.
			if len(ranges) < lineCount {
				ranges = append(ranges, rng{start: lineStart, end: len(contentStr)})
			}

			rangeStart := ranges[startLine-1].start
			rangeEnd := ranges[endLine-1].end
			contentRange := contentStr[rangeStart:rangeEnd]

			if isJSONOutput() {
				res := FileResult{
					Path:      relPath,
					Content:   contentRange,
					LineCount: lineCount,
				}
				// Only include range metadata when it's not the full file, or when lines are requested.
				if startLine != 1 || endLine != lineCount || readLinesFlag {
					res.StartLine = startLine
					res.EndLine = endLine
				}
				if readLinesFlag {
					lines := make([]FileLine, 0, endLine-startLine+1)
					for n := startLine; n <= endLine; n++ {
						seg := contentStr[ranges[n-1].start:ranges[n-1].end]
						seg = strings.TrimSuffix(seg, "\n")
						lines = append(lines, FileLine{Num: n, Text: seg})
					}
					res.Lines = lines
				}
				outputSuccess(res, &Meta{QueryTimeMs: elapsed})
				return nil
			}

			// Human-readable: just output the content
			fmt.Print(contentRange)
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
	readCmd.Flags().BoolVar(&readLinesFlag, "lines", false, "Include structured lines with line numbers (requires --raw)")
	readCmd.Flags().IntVar(&readStartLine, "start-line", 0, "Start line (1-indexed, inclusive) for raw output")
	readCmd.Flags().IntVar(&readEndLine, "end-line", 0, "End line (1-indexed, inclusive) for raw output")
	rootCmd.AddCommand(readCmd)
}
