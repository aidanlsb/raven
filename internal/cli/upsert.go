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

var upsertCmd = newCanonicalLeafCommand("upsert", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Prepare:     prepareUpsertValidation,
	Invoke:      invokeUpsertStdin,
	RenderHuman: renderUpsertResult,
})

func prepareUpsertValidation(cmd *cobra.Command, args []string) ([]string, bool, error) {
	// Validate title
	if len(args) > 0 {
		if err := validateObjectTitle(args[0]); err != nil {
			return nil, false, handleErrorMsg("INVALID_INPUT", err.Error(), "Provide a non-empty title")
		}
	}

	// Validate object-path if explicitly provided
	if cmd.Flags().Changed("object-path") {
		objectPath, _ := cmd.Flags().GetString("object-path")
		if err := validateObjectPath(objectPath); err != nil {
			return nil, false, handleErrorMsg("INVALID_INPUT", err.Error(), "Use --object-path with an object path like note/raven-friction (no type/ prefix, no .md suffix)")
		}
	}

	return args, false, nil
}

func invokeUpsertStdin(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Handle stdin content-file special case: read from stdin and replace content-file with content
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
