package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
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

		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "read",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"path":       reference,
				"raw":        readRawFlag,
				"lines":      readLinesFlag,
				"start-line": readStartLine,
				"end-line":   readEndLine,
			},
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(mapReadCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		elapsed := time.Since(start).Milliseconds()
		data, _ := result.Data.(map[string]interface{})

		if readRawFlag {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}

			// Human-readable: just output the content
			content, _ := data["content"].(string)
			fmt.Print(content)
			return nil
		}

		return readEnriched(readEnrichedOptions{
			fileRelPath:    stringFromMap(data, "path"),
			content:        stringFromMap(data, "content"),
			lineCount:      intFromMap(data, "line_count"),
			elapsedMs:      elapsed,
			references:     readReferencesFromMap(data["references"]),
			backlinks:      readBacklinksFromMap(data["backlinks"]),
			backlinksCount: metaCount(result.Meta),
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

func mapReadCode(code string) string {
	switch code {
	case "CONFIG_INVALID":
		return ErrConfigInvalid
	case "REF_AMBIGUOUS":
		return ErrRefAmbiguous
	case "REF_NOT_FOUND":
		return ErrRefNotFound
	case "INVALID_ARGS", "INVALID_INPUT":
		return ErrInvalidInput
	case "DATABASE_ERROR":
		return ErrDatabaseError
	case "FILE_NOT_FOUND":
		return ErrFileNotFound
	case "FILE_READ_ERROR":
		return ErrFileReadError
	default:
		return ErrInternal
	}
}

func stringFromMap(data map[string]interface{}, key string) string {
	if data == nil {
		return ""
	}
	value, _ := data[key].(string)
	return value
}

func intFromMap(data map[string]interface{}, key string) int {
	if data == nil {
		return 0
	}
	switch value := data[key].(type) {
	case int:
		return value
	case int64:
		return int(value)
	case float64:
		return int(value)
	default:
		return 0
	}
}

func metaCount(meta *commandexec.Meta) int {
	if meta == nil {
		return 0
	}
	return meta.Count
}

func readReferencesFromMap(raw interface{}) []readsvc.ReadReference {
	refs, ok := raw.([]readsvc.ReadReference)
	if ok {
		return refs
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	refs = make([]readsvc.ReadReference, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		ref := readsvc.ReadReference{}
		ref.Text, _ = entry["text"].(string)
		if path, ok := entry["path"].(string); ok {
			ref.Path = &path
		}
		refs = append(refs, ref)
	}
	return refs
}

func readBacklinksFromMap(raw interface{}) []readsvc.ReadBacklinkGroup {
	backlinks, ok := raw.([]readsvc.ReadBacklinkGroup)
	if ok {
		return backlinks
	}

	rows, ok := raw.([]interface{})
	if !ok {
		return nil
	}

	backlinks = make([]readsvc.ReadBacklinkGroup, 0, len(rows))
	for _, row := range rows {
		entry, ok := row.(map[string]interface{})
		if !ok {
			continue
		}
		group := readsvc.ReadBacklinkGroup{}
		group.Source, _ = entry["source"].(string)
		group.Lines = stringSliceFromAny(entry["lines"])
		backlinks = append(backlinks, group)
	}
	return backlinks
}

func stringSliceFromAny(raw interface{}) []string {
	switch values := raw.(type) {
	case []string:
		return values
	case []interface{}:
		out := make([]string, 0, len(values))
		for _, value := range values {
			text, ok := value.(string)
			if ok {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}
