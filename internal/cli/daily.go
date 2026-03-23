package cli

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vault"
)

var dailyEdit bool
var dailyTemplate string

var dailyCmd = &cobra.Command{
	Use:   "daily [date]",
	Short: "Open or create a daily note",
	Long: `Creates a daily note if it doesn't exist.

If no date is provided, defaults to today.
Use --edit to open it in your editor.
Use --template to select a specific core date template ID.

Examples:
  rvn daily              # Today's note
  rvn daily yesterday    # Yesterday's note
  rvn daily tomorrow     # Tomorrow's note
  rvn daily 2025-02-01   # Specific date
  rvn daily 2025-02-01 --template daily_brief
  rvn daily --edit       # Open in editor`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		var dateArg string
		if len(args) > 0 {
			dateArg = args[0]
		}
		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "daily",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"date":     dateArg,
				"template": dailyTemplate,
			},
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

		data, _ := result.Data.(map[string]interface{})
		relativePath, _ := data["file"].(string)
		created, _ := data["created"].(bool)
		filePath := relativePath
		if relativePath != "" {
			filePath = filepath.Join(vaultPath, filepath.FromSlash(relativePath))
		}

		if !isJSONOutput() && created {
			fmt.Printf("Created %s\n", relativePath)
		}

		if isJSONOutput() {
			editor := ""
			if cfg := getConfig(); cfg != nil {
				editor = cfg.GetEditor()
			}

			opened := false
			if dailyEdit {
				opened = vault.OpenInEditor(getConfig(), filePath)
			}

			payload := map[string]interface{}{}
			for key, value := range data {
				payload[key] = value
			}
			payload["opened"] = opened
			payload["editor"] = editor
			outputSuccess(payload, result.Meta)
			return nil
		}

		openFileInEditor(filePath, relativePath, created)

		return nil
	},
}

func init() {
	dailyCmd.Flags().BoolVarP(&dailyEdit, "edit", "e", false, "Open the note in the configured editor")
	dailyCmd.Flags().StringVar(&dailyTemplate, "template", "", "Core date template ID to use when creating a new daily note")
	rootCmd.AddCommand(dailyCmd)
}
