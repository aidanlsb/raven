package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
)

var readCmd = newCanonicalLeafCommand("read", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Args:        cobra.MaximumNArgs(1),
	Prepare:     prepareReadArgs,
	BuildArgs:   buildReadArgs,
	HandleError: handleCanonicalReadFailure,
	RenderHuman: renderRead,
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

	if canUseFZFInteractive() {
		selectedPath, selected, err := pickVaultFileWithFZF(vaultPath, vaultCfg, "read> ", "Select a file to read (Esc to cancel)")
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
	if lines || startLine > 0 || endLine > 0 {
		raw = true
	}
	return map[string]interface{}{
		"path":       args[0],
		"raw":        raw,
		"lines":      lines,
		"start-line": startLine,
		"end-line":   endLine,
	}, nil
}

func handleCanonicalReadFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	return handleErrorWithDetails(mapReadCode(result.Error.Code), result.Error.Message, result.Error.Suggestion, result.Error.Details)
}

func renderRead(cmd *cobra.Command, result commandexec.Result) error {
	raw, _ := cmd.Flags().GetBool("raw")
	lines, _ := cmd.Flags().GetBool("lines")
	startLine, _ := cmd.Flags().GetInt("start-line")
	endLine, _ := cmd.Flags().GetInt("end-line")
	rawMode := raw || lines || startLine > 0 || endLine > 0

	data := canonicalDataMap(result)
	if rawMode {
		content, _ := data["content"].(string)
		fmt.Print(content)
		return nil
	}

	return readEnriched(readEnrichedOptions{
		fileRelPath:    stringFromMap(data, "path"),
		content:        stringFromMap(data, "content"),
		lineCount:      intFromMap(data, "line_count"),
		elapsedMs:      queryTimeMs(result.Meta),
		references:     readReferencesFromMap(data["references"]),
		backlinks:      readBacklinksFromMap(data["backlinks"]),
		backlinksCount: metaCount(result.Meta),
	})
}

func init() {
	readCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    false,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
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

func queryTimeMs(meta *commandexec.Meta) int64 {
	if meta == nil {
		return 0
	}
	return meta.QueryTimeMs
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
