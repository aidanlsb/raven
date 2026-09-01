package cli

import (
	"encoding/json"
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
	FlagBindings: map[string]interface{}{
		"field":        &upsertFieldFlags,
		"fields-json":  &upsertFieldJSON,
		"content":      &upsertContent,
		"content-file": &upsertContentFile,
		"object-path":  &upsertObjectPath,
	},
})

func invokeUpsert(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	// Test compatibility: merge global flag values
	if _, hasField := args["field"]; !hasField && len(upsertFieldFlags) > 0 {
		parsed, _ := parseKeyValueArgs("field", upsertFieldFlags)
		args["field"] = parsed
	}
	if _, hasFieldsJSON := args["fields-json"]; !hasFieldsJSON && upsertFieldJSON != "" {
		var decoded interface{}
		if err := json.Unmarshal([]byte(upsertFieldJSON), &decoded); err == nil {
			args["fields-json"] = decoded
		}
	}
	if _, hasContent := args["content"]; !hasContent && upsertContent != "" {
		args["content"] = upsertContent
	}
	if _, hasContentFile := args["content-file"]; !hasContentFile && upsertContentFile != "" {
		args["content-file"] = upsertContentFile
	}
	if _, hasObjPath := args["object-path"]; !hasObjPath && upsertObjectPath != "" {
		args["object-path"] = upsertObjectPath
	}

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
			return commandexec.Failure("FILE_READ", "failed to read content from stdin", err, "")
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
		fmt.Println(ui.Checkf("Created %s", ui.FilePath(data.File)))
	case "updated":
		fmt.Println(ui.Checkf("Updated %s", ui.FilePath(data.File)))
	default:
		fmt.Println(ui.Checkf("Unchanged %s", ui.FilePath(data.File)))
	}
	if data.ID != "" {
		fmt.Println(ui.LinkAs(data.ID))
	}
	for _, warning := range result.Warnings {
		fmt.Println(ui.Warning(warning.Message))
	}
	if data.Status == "created" || data.Status == "updated" {
		promptCreateMissingRefsFromResult(getVaultPath(), result)
	}
	return nil
}

func parseFieldFlags(flags []string) (map[string]string, error) {
	fields := make(map[string]string, len(flags))
	for _, f := range flags {
		parts := strings.SplitN(f, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid field format: %s", f)
		}
		fields[parts[0]] = parts[1]
	}
	return fields, nil
}

func init() {
	rootCmd.AddCommand(upsertCmd)
}
