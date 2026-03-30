package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	addToFlag      string
	addHeadingFlag string
	addStdin       bool
	addConfirm     bool
)

var addCmd = newCanonicalLeafCommand("add", canonicalLeafOptions{
	VaultPath:       getVaultPath,
	Args:            cobra.ArbitraryArgs,
	BuildArgs:       buildAddArgs,
	Invoke:          invokeAdd,
	RenderHuman:     renderAddResult,
	SkipFlagBinding: true,
})

func buildAddArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	if addStdin {
		if len(args) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no text to add", "Usage: rvn add --stdin <text>")
		}
		text := strings.Join(args, " ")

		fileIDs, embeddedIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, handleError(ErrInternal, err, "")
		}
		ids := append(fileIDs, embeddedIDs...)
		if len(ids) == 0 {
			return nil, handleErrorMsg(ErrMissingArgument, "no object IDs provided via stdin", "Pipe object IDs to stdin, one per line")
		}

		argsMap := map[string]interface{}{
			"text":       formatCaptureLine(text),
			"stdin":      true,
			"object_ids": stringsToAny(ids),
		}
		if headingSpec := effectiveAddHeadingSpec(); headingSpec != "" {
			argsMap["heading"] = headingSpec
		}
		return argsMap, nil
	}

	if len(args) == 0 {
		return nil, handleErrorMsg(ErrMissingArgument, "requires text argument", "Usage: rvn add <text>")
	}

	text := strings.Join(args, " ")
	argsMap := map[string]interface{}{
		"text": formatCaptureLine(text),
	}
	if to := strings.TrimSpace(addToFlag); to != "" {
		argsMap["to"] = to
	}
	if headingSpec := effectiveAddHeadingSpec(); headingSpec != "" {
		argsMap["heading"] = headingSpec
	}
	return argsMap, nil
}

func invokeAdd(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   addConfirm,
	})
}

func renderAddResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(map[string]interface{})
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	if boolValue(data["bulk"]) || boolValue(data["stdin"]) {
		return renderCanonicalBulkResult(result)
	}

	relativePath, _ := data["file"].(string)
	fmt.Println(ui.Checkf("Added to %s", ui.FilePath(relativePath)))
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warningf("%s: %s", warning.Code, warning.Message))
		if warning.CreateCommand != "" {
			fmt.Printf("    %s\n", ui.Hint("→ "+warning.CreateCommand))
		}
	}
	return nil
}

func formatCaptureLine(text string) string {
	return text
}

func effectiveAddHeadingSpec() string {
	return strings.TrimSpace(addHeadingFlag)
}

func parseHeadingTextFromSpec(spec string) (string, bool) {
	trimmed := strings.TrimSpace(spec)
	if !strings.HasPrefix(trimmed, "#") {
		return "", false
	}
	i := 0
	for i < len(trimmed) && trimmed[i] == '#' {
		i++
	}
	if i == 0 || i >= len(trimmed) || trimmed[i] != ' ' {
		return "", false
	}
	headingText := strings.TrimSpace(trimmed[i:])
	if headingText == "" {
		return "", false
	}
	return headingText, true
}

func buildCreateObjectCommand(typeName, targetRaw string) string {
	title := filepath.Base(strings.TrimSpace(targetRaw))
	if title == "" || title == "." || title == "/" {
		title = "new-object"
	}
	return fmt.Sprintf("rvn new %s %q --json", typeName, title)
}

func init() {
	addCmd.Flags().StringVar(&addToFlag, "to", "", "Target file (path or reference like 'cursor')")
	addCmd.Flags().StringVar(&addHeadingFlag, "heading", "", "Target heading within destination (heading slug, object#heading ID, or markdown heading text)")
	addCmd.Flags().BoolVar(&addStdin, "stdin", false, "Read object IDs from stdin (one per line)")
	addCmd.Flags().BoolVar(&addConfirm, "confirm", false, "Apply changes (without this flag, shows preview only)")
	if err := addCmd.RegisterFlagCompletionFunc("to", completeReferenceFlag(true)); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(addCmd)
}
