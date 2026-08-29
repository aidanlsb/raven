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
	BuildArgs:   buildUpsertArgs,
	RenderHuman: renderUpsertResult,
	FlagBindings: map[string]interface{}{
		"field":        &upsertFieldFlags,
		"fields-json":  &upsertFieldJSON,
		"content":      &upsertContent,
		"content-file": &upsertContentFile,
		"object-path":  &upsertObjectPath,
	},
})

func buildUpsertArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	typeName := args[0]
	title := args[1]

	if err := validateObjectTitle(title); err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Provide a non-empty title")
	}

	// Leave targetPath empty when no explicit --object-path is given so the service
	// derives the filename/slug from the title (which may contain "/").
	targetPath := ""
	if cmd.Flags().Changed("object-path") {
		targetPath = strings.TrimSpace(upsertObjectPath)
		if err := validateObjectPath(targetPath); err != nil {
			return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Use --object-path with an object path like note/raven-friction (no type/ prefix, no .md suffix). Use data.id from rvn read, not data.path.")
		}
	}

	fieldValues, err := parseFieldFlags(upsertFieldFlags)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
	}

	fieldJSONRaw, err := parseFieldJSONObject(upsertFieldJSON)
	if err != nil {
		return nil, handleErrorMsg(ErrInvalidInput, "invalid --fields-json payload", "Provide a JSON object, e.g. --fields-json '{\"status\":\"active\"}'")
	}

	content := upsertContent
	replaceBody := cmd.Flags().Changed("content")
	contentFileChanged := cmd.Flags().Changed("content-file")
	if replaceBody && contentFileChanged {
		return nil, handleErrorMsg(ErrInvalidInput, "--content and --content-file are mutually exclusive", "Use only one body input mode")
	}
	if contentFileChanged && strings.TrimSpace(upsertContentFile) == "-" {
		data, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, handleErrorMsg(ErrFileRead, "failed to read content from stdin", err.Error())
		}
		content = string(data)
		replaceBody = true
		contentFileChanged = false
	}

	return buildUpsertCommandArgs(
		typeName,
		title,
		targetPath,
		fieldValues,
		fieldJSONRaw,
		content,
		replaceBody,
		upsertContentFile,
		contentFileChanged,
	), nil
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

func buildUpsertCommandArgs(typeName, title, targetPath string, fieldValues map[string]string, fieldJSONRaw map[string]interface{}, content string, replaceBody bool, contentFile string, contentFileChanged bool) map[string]interface{} {
	args := map[string]interface{}{
		"type":  typeName,
		"title": title,
	}
	if len(fieldValues) > 0 {
		args["field"] = stringMapToAny(fieldValues)
	}
	if len(fieldJSONRaw) > 0 {
		args["fields-json"] = fieldJSONRaw
	}
	if strings.TrimSpace(targetPath) != "" {
		args["object-path"] = targetPath
	}
	if replaceBody {
		args["content"] = content
	}
	if contentFileChanged {
		args["content-file"] = strings.TrimSpace(contentFile)
	}
	return args
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
