package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	upsertFieldFlags []string
	upsertFieldJSON  string
	upsertContent    string
	upsertPathFlag   string
)

var upsertCmd = &cobra.Command{
	Use:   "upsert <type> <title>",
	Short: commands.Registry["upsert"].Description,
	Long:  commands.Registry["upsert"].LongDesc,
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		typeName := args[0]
		title := args[1]

		if err := validateObjectTitle(title); err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Provide a plain title without path separators")
		}

		targetPath := title
		if cmd.Flags().Changed("path") {
			targetPath = strings.TrimSpace(upsertPathFlag)
			if err := validateObjectTargetPath(targetPath); err != nil {
				return handleErrorMsg(ErrInvalidInput, err.Error(), "Use --path with an explicit object path like note/raven-friction")
			}
		}

		fieldValues, err := parseFieldFlags(upsertFieldFlags)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, err.Error(), "Use format: --field name=value")
		}

		fieldJSONRaw, err := parseFieldJSONObject(upsertFieldJSON)
		if err != nil {
			return handleErrorMsg(ErrInvalidInput, "invalid --field-json payload", "Provide a JSON object, e.g. --field-json '{\"status\":\"active\"}'")
		}

		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "upsert",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: buildUpsertCommandArgs(
				typeName,
				title,
				targetPath,
				fieldValues,
				fieldJSONRaw,
				upsertContent,
				cmd.Flags().Changed("content"),
			),
		})
		if !result.OK {
			if isJSONOutput() {
				outputJSON(result)
				return nil
			}
			if result.Error != nil {
				return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
			}
			return handleErrorMsg(ErrInternal, "command execution failed", "")
		}

		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		data, _ := result.Data.(map[string]interface{})
		status, _ := data["status"].(string)
		relativePath, _ := data["file"].(string)
		switch status {
		case "created":
			fmt.Println(ui.Checkf("Created %s", ui.FilePath(relativePath)))
		case "updated":
			fmt.Println(ui.Checkf("Updated %s", ui.FilePath(relativePath)))
		default:
			fmt.Println(ui.Checkf("Unchanged %s", ui.FilePath(relativePath)))
		}
		for _, warning := range result.Warnings {
			fmt.Println(ui.Warning(warning.Message))
		}
		return nil
	},
}

func buildUpsertCommandArgs(typeName, title, targetPath string, fieldValues map[string]string, fieldJSONRaw map[string]interface{}, content string, replaceBody bool) map[string]interface{} {
	args := map[string]interface{}{
		"type":  typeName,
		"title": title,
	}
	if len(fieldValues) > 0 {
		args["field"] = stringMapToAny(fieldValues)
	}
	if len(fieldJSONRaw) > 0 {
		args["field-json"] = fieldJSONRaw
	}
	if strings.TrimSpace(targetPath) != "" {
		args["path"] = targetPath
	}
	if replaceBody {
		args["content"] = content
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
	upsertCmd.Flags().StringArrayVar(&upsertFieldFlags, "field", nil, "Set field value (can be repeated): --field name=value")
	upsertCmd.Flags().StringVar(&upsertFieldJSON, "field-json", "", "Set/update frontmatter fields as a JSON object")
	upsertCmd.Flags().StringVar(&upsertContent, "content", "", "Replace body content (idempotent full-body mode)")
	upsertCmd.Flags().StringVar(&upsertPathFlag, "path", "", "Explicit target path (overrides title-derived path)")
	rootCmd.AddCommand(upsertCmd)
}
