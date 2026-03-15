package cli

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/readsvc"
)

var (
	readRawFlag     bool
	readNoLinksFlag bool
	readLinesFlag   bool
	readStartLine   int
	readEndLine     int
)

var readCmd = &cobra.Command{
	Use:   "read [reference]",
	Short: "Read a file with context",
	Long: `Read and output a file from the vault.

The reference can be a short reference (freya), partial path (people/freya),
or full path (people/freya.md).

By default, this command shows enriched output (rendered wikilinks and backlinks).
Use --raw to output only the raw file content (useful for agents and scripting).

In an interactive terminal with fzf installed, bare 'rvn read' launches
an interactive file picker.

Examples:
  rvn read freya                  # Resolves to people/freya.md
  rvn read people/freya
  rvn read daily/2025-02-01.md
  rvn read people/freya --raw
  rvn read people/freya --raw --lines
  rvn read people/freya --raw --start-line 10 --end-line 40
  rvn read people/freya --raw --json`,
	Args: cobra.MaximumNArgs(1),
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: false,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		start := time.Now()

		// Apply hyperlink preference for this run.
		setHyperlinksDisabled(readNoLinksFlag)

		// Load vault config
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		var reference string
		if len(args) == 0 {
			if canUseFZFInteractive() {
				selectedPath, selected, err := pickVaultFileWithFZF(vaultPath, vaultCfg, "read> ", "Select a file to read (Esc to cancel)")
				if err != nil {
					return handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
				}
				if !selected {
					return nil
				}
				reference = selectedPath
			} else {
				return handleErrorMsg(
					ErrMissingArgument,
					"specify a reference",
					interactivePickerMissingArgSuggestion("read", "rvn read <reference>"),
				)
			}
		} else {
			reference = args[0]
		}

		// If the caller requested line-based output/ranges, force raw mode so the content is stable.
		if readLinesFlag || readStartLine > 0 || readEndLine > 0 {
			readRawFlag = true
		}

		rt, err := readsvc.NewRuntime(vaultPath, readsvc.RuntimeOptions{OpenDB: false})
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		defer rt.Close()

		result, err := readsvc.Read(rt, readsvc.ReadRequest{
			Reference: reference,
			Raw:       readRawFlag,
			Lines:     readLinesFlag,
			StartLine: readStartLine,
			EndLine:   readEndLine,
		})
		if err != nil {
			var ambiguous *readsvc.AmbiguousRefError
			if errors.As(err, &ambiguous) {
				return handleErrorMsg(ErrRefAmbiguous, ambiguous.Error(), "Use a full object ID/path to disambiguate")
			}

			var notFound *readsvc.RefNotFoundError
			if errors.As(err, &notFound) {
				return handleErrorMsg(ErrRefNotFound, notFound.Error(), "Check the reference and try again")
			}

			var invalidRange *readsvc.InvalidLineRangeError
			if errors.As(err, &invalidRange) {
				return handleErrorMsg(ErrInvalidInput, invalidRange.Error(), invalidRange.Suggestion())
			}

			if os.IsNotExist(err) {
				return handleErrorMsg(ErrFileNotFound, err.Error(), "Check the path and try again")
			}

			if strings.Contains(err.Error(), "failed to open database") || strings.Contains(err.Error(), "failed to create resolver") {
				return handleError(ErrDatabaseError, err, "Run 'rvn reindex' to rebuild the database")
			}
			return handleError(ErrFileReadError, err, "")
		}

		elapsed := time.Since(start).Milliseconds()

		if readRawFlag {
			if isJSONOutput() {
				res := FileResult{
					Path:      result.Path,
					Content:   result.Content,
					LineCount: result.LineCount,
				}
				// Only include range metadata when it's not the full file, or when lines are requested.
				if result.StartLine > 0 {
					res.StartLine = result.StartLine
					res.EndLine = result.EndLine
				}
				if len(result.Lines) > 0 {
					lines := make([]FileLine, 0, len(result.Lines))
					for _, line := range result.Lines {
						lines = append(lines, FileLine{Num: line.Num, Text: line.Text})
					}
					res.Lines = lines
				}
				outputSuccess(res, &Meta{QueryTimeMs: elapsed})
				return nil
			}

			// Human-readable: just output the content
			fmt.Print(result.Content)
			return nil
		}

		return readEnriched(readEnrichedOptions{
			fileRelPath:    result.Path,
			content:        result.Content,
			lineCount:      result.LineCount,
			elapsedMs:      elapsed,
			references:     result.References,
			backlinks:      result.Backlinks,
			backlinksCount: result.BacklinksCount,
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
