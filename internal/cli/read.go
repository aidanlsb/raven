package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var readCmd = newCanonicalLeafCommand("read", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           cobra.MaximumNArgs(1),
	Prepare:        prepareReadArgs,
	BuildArgs:      buildReadArgs,
	HandleErrorCmd: handleCanonicalReadFailureCmd,
	RenderHuman:    renderRead,
})

func prepareReadArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	noLinks, _ := cmd.Flags().GetBool("no-links")
	setHyperlinksDisabled(noLinks)

	if len(args) > 0 {
		return args, false, nil
	}

	vaultPath := getVaultPath()
	vaultCfg, err := loadVaultConfigSafe(vaultPath)
	if err != nil {
		return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
	}

	if canUseRavenInteractive() {
		selectedPath, selected, err := cliSelector.vaultFile(vaultPath, vaultCfg, "read> ", "Select a file to read")
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
		}
		if !selected {
			return nil, true, nil
		}
		return []string{selectedPath}, false, nil
	}

	err = handleErrorMsg(
		ErrMissingArgument,
		"specify a reference",
		interactivePickerMissingArgSuggestion("read", "rvn read <reference>"),
	)
	return nil, err == nil, err
}

func buildReadArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	raw, _ := cmd.Flags().GetBool("raw")
	lines, _ := cmd.Flags().GetBool("lines")
	startLine, _ := cmd.Flags().GetInt("start-line")
	endLine, _ := cmd.Flags().GetInt("end-line")
	sections, _ := cmd.Flags().GetBool("sections")
	if lines || startLine > 0 || endLine > 0 {
		raw = true
	}
	return map[string]interface{}{
		"reference":  args[0],
		"raw":        raw,
		"lines":      lines,
		"start-line": startLine,
		"end-line":   endLine,
		"sections":   sections,
	}, nil
}

func handleCanonicalReadFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapReadCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func handleCanonicalReadFailureCmd(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "read",
		Prompt:    "read/ref> ",
		Fallback:  handleCanonicalReadFailure,
		BuildArgs: func(cmd *cobra.Command, selected string) (map[string]interface{}, error) {
			return buildReadArgs(cmd, []string{selected})
		},
		Render: renderRead,
	})
}

func renderRead(cmd *cobra.Command, result commandexec.Result) error {
	sections, _ := cmd.Flags().GetBool("sections")
	if sections {
		payload, ok := result.Data.(commandpayload.ReadSectionsResult)
		if !ok {
			return handleErrorMsg(ErrInternal, "unexpected read result shape", "")
		}
		renderReadSections(payload)
		return nil
	}

	if payload, ok := result.Data.(commandpayload.ReadRawResult); ok {
		fmt.Print(payload.Content)
		return nil
	}

	payload, ok := result.Data.(commandpayload.ReadContentResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "unexpected read result shape", "")
	}
	return readEnriched(readEnrichedOptions{
		fileRelPath:    payload.Path,
		content:        payload.Content,
		lineCount:      payload.LineCount,
		elapsedMs:      queryTimeMs(result.Meta),
		references:     payload.References,
		backlinks:      payload.Backlinks,
		backlinksCount: metaCount(result.Meta),
	})
}

func renderReadSections(payload commandpayload.ReadSectionsResult) {
	if payload.Path != "" {
		fmt.Println(ui.FilePath(payload.Path))
	}
	if len(payload.Sections) == 0 {
		fmt.Println(ui.Hint("No sections found."))
		return
	}
	for _, entry := range payload.Sections {
		level := entry.Level
		if level < 1 {
			level = 1
		}
		indent := strings.Repeat("  ", level-1)
		fmt.Printf("%s%s %s\n", indent, entry.Title, ui.Hint(fmt.Sprintf("(%s, line %d)", entry.ID, entry.LineStart)))
	}
}

func init() {
	readCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(readCmd)
}

func mapReadCode(code codes.ErrorCode) codes.ErrorCode {
	switch code {
	case codes.ErrConfigInvalid:
		return ErrConfigInvalid
	case codes.ErrRefAmbiguous:
		return ErrRefAmbiguous
	case codes.ErrRefNotFound:
		return ErrRefNotFound
	case codes.ErrInvalidArgs, codes.ErrInvalidInput:
		return ErrInvalidInput
	case codes.ErrDatabase:
		return ErrDatabase
	case codes.ErrFileNotFound:
		return ErrFileNotFound
	case codes.ErrFileRead:
		return ErrFileRead
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

func queryTimeMs(meta *commandexec.Meta) int64 {
	if meta == nil {
		return 0
	}
	return meta.QueryTimeMs
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
