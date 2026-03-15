package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/datesvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var dateCmd = &cobra.Command{
	Use:   "date [date]",
	Short: "Show everything related to a date",
	Long: `Shows all objects and traits associated with a specific date.

This includes:
- The daily note for that date (if exists)
- Any trait or field with a date value matching that date (e.g., @due, @remind, event dates)
- References to that date (e.g., [[2025-02-01]])

Examples:
  rvn date              # Today
  rvn date yesterday
  rvn date 2025-02-01`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		var dateArg string
		if len(args) > 0 {
			dateArg = args[0]
		}
		result, err := datesvc.DateHub(datesvc.DateHubRequest{
			VaultPath: vaultPath,
			DateArg:   dateArg,
		})
		if err != nil {
			return mapDateSvcError(err)
		}

		if isJSONOutput() {
			data := map[string]interface{}{
				"date":          result.Date,
				"day_of_week":   result.DayOfWeek,
				"daily_note_id": result.DailyNoteID,
				"daily_path":    result.DailyPath,
				"daily_exists":  result.DailyExists,
				"items":         result.Items,
				"backlinks":     result.Backlinks,
			}
			if result.DailyNote != nil {
				data["daily_note"] = result.DailyNote
			}
			outputSuccess(data, &Meta{Count: len(result.Items)})
			return nil
		}

		display := ui.NewDisplayContext()
		fmt.Printf("%s %s\n\n", ui.SectionHeader(result.Date), ui.Hint(fmt.Sprintf("(%s)", result.DayOfWeek)))

		fmt.Println(ui.Divider("Daily Note", display.TermWidth))
		if result.DailyNote != nil {
			fmt.Printf("%s\n\n", ui.Bullet(ui.FilePath(result.DailyNote.FilePath)))
		} else {
			fmt.Printf("%s\n\n", ui.Bullet(ui.Hint(fmt.Sprintf("(not created yet - use 'rvn daily %s' to create)", result.Date))))
		}

		byField := make(map[string][]datesvc.DateAssociation)
		for _, item := range result.Items {
			byField[item.FieldName] = append(byField[item.FieldName], item)
		}

		for fieldName, fieldItems := range byField {
			prettyField := fieldName
			if prettyField != "" {
				prettyField = strings.ToUpper(prettyField[:1]) + prettyField[1:]
			}
			label := fmt.Sprintf("%s: %s (%d)", prettyField, result.Date, len(fieldItems))
			fmt.Println(ui.Divider(label, display.TermWidth))
			for _, item := range fieldItems {
				if item.SourceType == "trait" {
					if item.Trait != nil {
						valueStr := ""
						if item.Trait.Value != nil && *item.Trait.Value != "" {
							valueStr = *item.Trait.Value
						}
						line := fmt.Sprintf("%s %s", ui.Trait(item.Trait.TraitType, valueStr), item.Trait.Content)
						fmt.Println(ui.Bullet(line))
						fmt.Println(ui.Indent(2, ui.Hint(item.Trait.FilePath)))
					} else {
						fmt.Println(ui.Bullet(item.SourceID))
					}
				} else {
					if item.Object != nil {
						meta := ""
						if item.Object.Type != "" {
							meta = ui.Hint(fmt.Sprintf("(%s)", item.Object.Type))
						}
						fmt.Println(ui.Bullet(strings.TrimSpace(fmt.Sprintf("%s %s", item.SourceID, meta))))
					} else {
						fmt.Println(ui.Bullet(item.SourceID))
					}
				}
			}
			fmt.Println()
		}

		if len(result.Backlinks) > 0 {
			fmt.Println(ui.Divider(fmt.Sprintf("References (%d)", len(result.Backlinks)), display.TermWidth))
			for _, bl := range result.Backlinks {
				location := bl.FilePath
				if bl.Line != nil {
					location = fmt.Sprintf("%s:%d", bl.FilePath, *bl.Line)
				}
				fmt.Println(ui.Bullet(ui.Hint(location)))
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(dateCmd)
}
