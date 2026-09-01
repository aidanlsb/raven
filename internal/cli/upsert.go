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

var (
	upsertFieldFlags  []string
	upsertFieldJSON   string
	upsertContent     string
	upsertContentFile string
	upsertObjectPath  string
)

var upsertCmd = newCanonicalLeafCommand("upsert", canonicalLeafOptions{
	VaultPath:   getVaultPath,
	Invoke:      invokeUpsert,
	RenderHuman: renderUpsertResult,
})

func invokeUpsert(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Validate title
	title := stringValue(args["title"])
	if err := validateObjectTitle(title); err != nil {
		return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Provide a non-empty title")
	}

	// Validate object-path if explicitly provided
	if cmd.Flags().Changed("object-path") || upsertObjectPath != "" {
		targetPath := stringValue(args["object-path"])
		if err := validateObjectPath(targetPath); err != nil {
			return commandexec.Failure("INVALID_INPUT", err.Error(), nil, "Use --object-path with an object path like note/raven-friction (no type/ prefix, no .md suffix)")
		}
	}

	// Handle stdin content-file
	contentFileChanged := cmd.Flags().Changed("content-file") || upsertContentFile != ""
	if contentFileChanged && strings.TrimSpace(stringValue(args["content-file"])) == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return commandexec.Failure("FILE_READ_ERROR", "failed to read content from stdin", err, "")
		}
		args["content"] = string(data)
		delete(args, "content-file")
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
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
		renderObjectUpdated(data.File)
		if data.ID != "" {
			fmt.Println(ui.LinkAs(data.ID))
		}
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
