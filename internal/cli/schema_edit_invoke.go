package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/commandpayload"
	"github.com/aidanlsb/raven/internal/commands"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/ui"
)

func buildSchemaArgs(commandID string, cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	meta, ok := commands.EffectiveMeta(commandID)
	if !ok {
		return nil, fmt.Errorf("registry metadata missing for %q", commandID)
	}
	return buildCanonicalArgsForMeta(meta, cmd, args)
}

func invokeSchemaAddType(cmd *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	nameField, _ := cmd.Flags().GetString("name-field")
	if strings.TrimSpace(nameField) == "" && !isJSONOutput() {
		fmt.Print("Which field should be the display name? (common: name, title; leave blank for none): ")
		var input string
		fmt.Scanln(&input)
		nameField = strings.TrimSpace(input)
		args["name-field"] = nameField
	}

	defaultPath, _ := cmd.Flags().GetString("default-path")
	if strings.TrimSpace(defaultPath) == "" {
		typeName := stringValue(args["name"])
		args["default-path"] = paths.NormalizeDirRoot(typeName)
	}

	return executeCanonicalRequest(commandexec.Request{
		CommandID: commandID,
		VaultPath: vaultPath,
		Args:      args,
	})
}

func invokeSchemaRemoveType(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	result := executeCanonicalCommand(commandID, vaultPath, args)
	if isJSONOutput() || boolValue(args["force"]) || result.OK || result.Error == nil || result.Error.Code != ErrConfirmationRequired {
		return result
	}

	details, _ := result.Error.Details.(map[string]interface{})
	count := detailInt(details, "affected_count")
	typeName := stringValue(args["name"])
	if count > 0 {
		fmt.Println(ui.Warningf("%d files of type '%s' will become 'page' type:", count, typeName))
		for _, filePath := range detailStringSlice(details, "affected_files") {
			fmt.Println(ui.Bullet(ui.FilePath(filePath)))
		}
		if remaining := detailInt(details, "remaining_count"); remaining > 0 {
			fmt.Printf("  %s\n", ui.Hint(fmt.Sprintf("... and %d more", remaining)))
		}
	}
	if !promptForConfirm("Continue?") {
		return commandexec.Failure(ErrConfirmationRequired, "operation cancelled", nil, "Use --force to skip confirmation")
	}

	retry := cloneArgsMap(args)
	retry["force"] = true
	return executeCanonicalCommand(commandID, vaultPath, retry)
}

func invokeSchemaRemoveTrait(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	result := executeCanonicalCommand(commandID, vaultPath, args)
	if isJSONOutput() || boolValue(args["force"]) || result.OK || result.Error == nil || result.Error.Code != ErrConfirmationRequired {
		return result
	}

	details, _ := result.Error.Details.(map[string]interface{})
	count := detailInt(details, "affected_count")
	traitName := stringValue(args["name"])
	if count > 0 {
		fmt.Println(ui.Warningf("%d instances of @%s will remain in files (no longer indexed)", count, traitName))
	}
	if !promptForConfirm("Continue?") {
		return commandexec.Failure(ErrConfirmationRequired, "operation cancelled", nil, "Use --force to skip confirmation")
	}

	retry := cloneArgsMap(args)
	retry["force"] = true
	return executeCanonicalCommand(commandID, vaultPath, retry)
}

func invokeSchemaRenameType(_ *cobra.Command, commandID, vaultPath string, args map[string]interface{}) commandexec.Result {
	confirm := boolValue(args["confirm"])

	previewArgs := map[string]interface{}{
		"old_name": args["old_name"],
		"new_name": args["new_name"],
	}
	if description, ok := args["description"]; ok {
		previewArgs["description"] = description
	}
	preview := executeCanonicalCommand(commandID, vaultPath, previewArgs)
	if !preview.OK {
		return preview
	}
	if !confirm {
		return preview
	}

	previewData, ok := preview.Data.(commandpayload.SchemaRenameTypePreviewResult)
	if !ok {
		return commandexec.Failure(ErrInternal, fmt.Sprintf("unexpected schema rename preview payload %T", preview.Data), nil, "")
	}
	applyDefaultPathRename := boolValue(args["rename-default-path"])
	defaultPathOld, defaultPathNew, filesToMove, renameAvailable := schemaRenameDefaultPathPreview(previewData)
	if renameAvailable && !applyDefaultPathRename && shouldPromptForConfirm() {
		prompt := fmt.Sprintf("Also rename default_path '%s' -> '%s'?", defaultPathOld, defaultPathNew)
		if filesToMove > 0 {
			prompt = fmt.Sprintf(
				"Also rename default_path '%s' -> '%s' and move %d files with reference updates?",
				defaultPathOld,
				defaultPathNew,
				filesToMove,
			)
		}
		applyDefaultPathRename = promptForConfirm(prompt)
	}

	applyArgs := cloneArgsMap(args)
	applyArgs["rename-default-path"] = applyDefaultPathRename
	applyArgs["confirm"] = true
	return executeCanonicalCommand(commandID, vaultPath, applyArgs)
}

func schemaRenameDefaultPathPreview(data commandpayload.SchemaRenameTypePreviewResult) (oldPath, newPath string, filesToMove int, available bool) {
	if data.DefaultPathRenameAvailable == nil || !*data.DefaultPathRenameAvailable {
		return "", "", 0, false
	}
	if data.DefaultPathOld != nil {
		oldPath = *data.DefaultPathOld
	}
	if data.DefaultPathNew != nil {
		newPath = *data.DefaultPathNew
	}
	if data.FilesToMove != nil {
		filesToMove = *data.FilesToMove
	}
	return oldPath, newPath, filesToMove, true
}
