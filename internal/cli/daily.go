package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/datesvc"
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
		result, err := datesvc.EnsureDaily(datesvc.EnsureDailyRequest{
			VaultPath:  vaultPath,
			DateArg:    dateArg,
			TemplateID: dailyTemplate,
		})
		if err != nil {
			return mapDateSvcError(err)
		}

		if !isJSONOutput() && result.Created {
			fmt.Printf("Created %s\n", result.RelativePath)
		}

		if isJSONOutput() {
			editor := ""
			if cfg := getConfig(); cfg != nil {
				editor = cfg.GetEditor()
			}

			opened := false
			if dailyEdit {
				opened = vault.OpenInEditor(getConfig(), result.FilePath)
			}

			outputSuccess(map[string]interface{}{
				"file":    result.RelativePath,
				"date":    result.Date,
				"created": result.Created,
				"opened":  opened,
				"editor":  editor,
			}, nil)
			return nil
		}

		openFileInEditor(result.FilePath, result.RelativePath, result.Created)

		return nil
	},
}

func mapDateSvcError(err error) error {
	svcErr, ok := datesvc.AsError(err)
	if !ok {
		return handleError(ErrInternal, err, "")
	}

	switch svcErr.Code {
	case datesvc.CodeInvalidInput:
		return handleErrorMsg(ErrInvalidInput, svcErr.Message, svcErr.Suggestion)
	case datesvc.CodeConfigInvalid:
		return handleError(ErrConfigInvalid, svcErr, svcErr.Suggestion)
	case datesvc.CodeSchemaInvalid:
		return handleError(ErrSchemaInvalid, svcErr, svcErr.Suggestion)
	case datesvc.CodeDatabaseError:
		return handleError(ErrDatabaseError, svcErr, svcErr.Suggestion)
	case datesvc.CodeQueryFailed:
		return handleError(ErrDatabaseError, svcErr, svcErr.Suggestion)
	case datesvc.CodeFileWriteErr:
		return handleError(ErrFileWriteError, svcErr, svcErr.Suggestion)
	default:
		return handleError(ErrInternal, svcErr, svcErr.Suggestion)
	}
}

func init() {
	dailyCmd.Flags().BoolVarP(&dailyEdit, "edit", "e", false, "Open the note in the configured editor")
	dailyCmd.Flags().StringVar(&dailyTemplate, "template", "", "Core date template ID to use when creating a new daily note")
	rootCmd.AddCommand(dailyCmd)
}
