package cli

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/app"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/datesvc"
	"github.com/aidanlsb/raven/internal/model"
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
		result := app.CommandInvoker().Execute(context.Background(), commandexec.Request{
			CommandID: "date",
			VaultPath: vaultPath,
			Caller:    commandexec.CallerCLI,
			Args: map[string]interface{}{
				"date": dateArg,
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
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}

		display := ui.NewDisplayContext()
		dateValue, _ := data["date"].(string)
		dayOfWeek, _ := data["day_of_week"].(string)
		fmt.Printf("%s %s\n\n", ui.SectionHeader(dateValue), ui.Hint(fmt.Sprintf("(%s)", dayOfWeek)))

		fmt.Println(ui.Divider("Daily Note", display.TermWidth))
		if dailyNoteRaw, ok := data["daily_note"]; ok && dailyNoteRaw != nil {
			dailyNote := objectResultFromAny(dailyNoteRaw)
			fmt.Printf("%s\n\n", ui.Bullet(ui.FilePath(dailyNote.FilePath)))
		} else {
			fmt.Printf("%s\n\n", ui.Bullet(ui.Hint(fmt.Sprintf("(not created yet - use 'rvn daily %s' to create)", dateValue))))
		}

		byField := make(map[string][]datesvc.DateAssociation)
		for _, item := range dateAssociationsFromAny(data["items"]) {
			byField[item.FieldName] = append(byField[item.FieldName], item)
		}

		for fieldName, fieldItems := range byField {
			prettyField := fieldName
			if prettyField != "" {
				prettyField = strings.ToUpper(prettyField[:1]) + prettyField[1:]
			}
			label := fmt.Sprintf("%s: %s (%d)", prettyField, dateValue, len(fieldItems))
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

		backlinks := dateBacklinksFromAny(data["backlinks"])
		if len(backlinks) > 0 {
			fmt.Println(ui.Divider(fmt.Sprintf("References (%d)", len(backlinks)), display.TermWidth))
			for _, bl := range backlinks {
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

func dateAssociationsFromAny(raw interface{}) []datesvc.DateAssociation {
	var items []datesvc.DateAssociation
	_ = decodeResultData(raw, &items)
	return items
}

func dateBacklinksFromAny(raw interface{}) []model.Reference {
	var items []model.Reference
	_ = decodeResultData(raw, &items)
	return items
}

func objectResultFromAny(raw interface{}) model.Object {
	var item model.Object
	_ = decodeResultData(raw, &item)
	return item
}

func init() {
	rootCmd.AddCommand(dateCmd)
}
