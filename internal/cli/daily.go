package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/objectsvc"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var dailyCmd = newExceptionLeafCommand("daily", exceptionLeafOptions{
	HandleResult: handleDailyResult,
})

func handleDailyResult(cmd *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[objectsvc.DailyPayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	filePath := data.File
	if data.File != "" {
		filePath = filepath.Join(getVaultPath(), filepath.FromSlash(data.File))
	}

	if !isJSONOutput() && data.Created {
		fmt.Println(ui.Checkf("Created %s", ui.FilePath(data.File)))
		if data.ID != "" {
			fmt.Println(ui.LinkAs(data.ID))
		}
	}

	edit, _ := cmd.Flags().GetBool("edit")
	if isJSONOutput() {
		editor := ""
		if cfg := getConfig(); cfg != nil {
			editor = cfg.GetEditor()
		}

		opened := false
		if edit {
			opened = vault.OpenInEditor(getConfig(), filePath)
		}

		payload := map[string]interface{}{
			"created": data.Created,
			"date":    data.Date,
			"file":    data.File,
			"id":      data.ID,
			"opened":  opened,
			"editor":  editor,
		}
		return outputSuccess(payload, result.Meta)
	}

	openFileInEditor(filePath, data.File, data.Created)
	return nil
}

func init() {
	// `--edit` is a CLI-only affordance and is intentionally excluded from the
	// shared canonical command contract exposed to MCP callers.
	dailyCmd.Flags().BoolP("edit", "e", false, "Open the note in the configured editor (CLI only)")
	rootCmd.AddCommand(dailyCmd)
}
