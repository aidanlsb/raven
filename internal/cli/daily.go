package cli

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/keep"
	"github.com/aidanlsb/raven/internal/pages"
	"github.com/aidanlsb/raven/internal/schema"
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
		keepPath := getKeepPath()

		// Load keep config for daily directory
		keepCfg, err := config.LoadKeepConfig(keepPath)
		if err != nil {
			return fmt.Errorf("failed to load keep config: %w", err)
		}

		// Parse date argument
		var dateArg string
		if len(args) > 0 {
			dateArg = args[0]
		}
		targetDate, err := keep.ParseDateArg(dateArg)
		if err != nil {
			return err
		}

		dateStr := keep.FormatDateISO(targetDate)
		targetPath := path.Join(keepCfg.GetDailyDirectory(), dateStr)
		dailyPath := filepath.Join(keepPath, keepCfg.GetDailyDirectory(), dateStr+".md")

		// Check if daily note already exists, create if needed
		created := false
		if !pages.Exists(keepPath, targetPath) {
			friendlyDate := keep.FormatDateFriendly(targetDate)
			s, err := schema.Load(keepPath)
			if err != nil {
				return fmt.Errorf("failed to load schema: %w", err)
			}
			var result *pages.CreateResult
			if strings.TrimSpace(dailyTemplate) != "" {
				templateOverride, err := schema.ResolveTypeTemplateFile(s, "date", dailyTemplate)
				if err != nil {
					return handleErrorMsg(ErrInvalidInput, err.Error(), "Use `rvn schema core date template list` to see available template IDs")
				}
				result, err = pages.CreateDailyNoteWithTemplate(
					keepPath,
					keepCfg.GetDailyDirectory(),
					dateStr,
					friendlyDate,
					templateOverride,
					keepCfg.GetTemplateDirectory(),
				)
				if err != nil {
					return fmt.Errorf("failed to create daily note: %w", err)
				}
			} else {
				result, err = pages.CreateDailyNoteWithSchema(
					keepPath,
					keepCfg.GetDailyDirectory(),
					dateStr,
					friendlyDate,
					s,
					keepCfg.GetTemplateDirectory(),
				)
				if err != nil {
					return fmt.Errorf("failed to create daily note: %w", err)
				}
			}

			if !isJSONOutput() {
				fmt.Printf("Created %s\n", result.RelativePath)
			}
			dailyPath = result.FilePath
			created = true
		}

		// Open in editor using shared logic
		relPath, _ := filepath.Rel(keepPath, dailyPath)

		if isJSONOutput() {
			editor := ""
			if cfg := getConfig(); cfg != nil {
				editor = cfg.GetEditor()
			}

			opened := false
			if dailyEdit {
				opened = keep.OpenInEditor(getConfig(), dailyPath)
			}

			outputSuccess(map[string]interface{}{
				"file":    relPath,
				"date":    dateStr,
				"created": created,
				"opened":  opened,
				"editor":  editor,
			}, nil)
			return nil
		}

		// In human CLI mode, preserve the historical behavior:
		// open daily notes by default. --edit is still used by JSON/agent callers.
		openFileInEditor(dailyPath, relPath, created)

		return nil
	},
}

func init() {
	dailyCmd.Flags().BoolVarP(&dailyEdit, "edit", "e", false, "Open the note in the configured editor")
	dailyCmd.Flags().StringVar(&dailyTemplate, "template", "", "Core date template ID to use when creating a new daily note")
	rootCmd.AddCommand(dailyCmd)
}
