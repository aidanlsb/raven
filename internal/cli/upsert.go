package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/ui"
)

var upsertCmd = newExceptionLeafCommand("upsert", exceptionLeafOptions{
	Invoke:      invokeUpsert,
	RenderHuman: renderUpsertResult,
})

func invokeUpsert(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	title := stringValue(args["title"])
	if title != "" {
		if err := validateObjectTitle(title); err != nil {
			return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Provide a non-empty title")
		}
	}
	if cmd.Flags().Changed("object-path") {
		objectPath, _ := cmd.Flags().GetString("object-path")
		if err := validateObjectPath(objectPath); err != nil {
			return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --object-path with an object path like note/raven-friction (no type/ prefix, no .md suffix)")
		}
	}
	if cmd.Flags().Changed("content-file") {
		contentFile := stringValue(args["content-file"])
		if strings.TrimSpace(contentFile) == "-" {
			data, err := io.ReadAll(os.Stdin)
			if err != nil {
				return commandexec.Failure("FILE_READ_ERROR", "failed to read content from stdin", err, "")
			}
			args["content"] = string(data)
			delete(args, "content-file")
		}
	}
	return executeCanonicalCommand(commandID, vaultPath, args)
}

func renderUpsertResult(_ *cobra.Command, result commandexec.Result) error {
	data, ok := result.Data.(commandpayload.UpsertResult)
	if !ok {
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}
	switch data.Status {
	case "created":
		renderObjectCreated(data.File, data.ID)
	case "updated":
		renderObjectUpdatedWithID(data.File, data.ID)
	default:
		fmt.Println(ui.Checkf("Unchanged %s", ui.FilePath(data.File)))
		if data.ID != "" {
			fmt.Println(ui.LinkAs(data.ID))
		}
	}
	for _, warning := range result.Warnings {
		fmt.Println(ui.Warning(warning.Message))
	}
	if data.Status == "created" || data.Status == "updated" {
		promptCreateMissingRefsFromResult(getVaultPath(), result)
	}
	return nil
}

func init() {
	rootCmd.AddCommand(upsertCmd)
}
