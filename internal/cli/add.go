package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	addToFlag  string
	addStdin   bool
	addConfirm bool
)

var addCmd = newCanonicalLeafCommand("add", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeAdd,
	RenderHuman: renderAddResult,
	FlagBindings: map[string]interface{}{
		"to":      &addToFlag,
		"stdin":   &addStdin,
		"confirm": &addConfirm,
	},
})


func invokeAdd(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Test compatibility: merge global flag values
	if _, hasTo := args["to"]; !hasTo && addToFlag != "" {
		args["to"] = addToFlag
	}
	if _, hasStdin := args["stdin"]; !hasStdin {
		args["stdin"] = addStdin
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
		Confirm:   addConfirm,
	})
}

func renderAddResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.AddResult)
	if !ok {
		if _, bulk := result.Data.(commandpayload.AddBulkPreviewResult); bulk {
			return renderCanonicalBulkResult(result)
		}
		if _, bulk := result.Data.(commandpayload.AddBulkResult); bulk {
			return renderCanonicalBulkResult(result)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	fmt.Println(ui.Checkf("Added to %s", ui.FilePath(data.File)))
	for _, warning := range result.Warnings {
		fmt.Printf("  %s\n", ui.Warningf("%s: %s", warning.Code, warning.Message))
		if warning.CreateCommand != "" {
			fmt.Printf("    %s\n", ui.Hint("→ "+warning.CreateCommand))
		}
	}
	promptCreateMissingRefsFromResult(getVaultPath(), result)
	return nil
}

func buildCreateObjectCommand(typeName, targetRaw string) string {
	title := filepath.Base(strings.TrimSpace(targetRaw))
	if title == "" || title == "." || title == "/" {
		title = "new-object"
	}
	return fmt.Sprintf("rvn new %s %q --json", typeName, title)
}

func init() {
	addCmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		message := err.Error()
		if strings.Contains(message, "unknown flag: --heading") || strings.Contains(message, "unknown flag: --create-heading") {
			return handleErrorMsg(
				ErrInvalidInput,
				"rvn add only appends body content; it no longer accepts --heading or --create-heading",
				`Create the heading with 'rvn section create <file> "<title>" --level N', then append content with 'rvn add <text> --to <file#section>'`,
			)
		}
		return err
	})
	if err := addCmd.RegisterFlagCompletionFunc("to", completeReferenceFlag(true)); err != nil {
		panic(err)
	}
	rootCmd.AddCommand(addCmd)
}
