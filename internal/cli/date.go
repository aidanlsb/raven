package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/ui"
)

var dateCmd = newCanonicalLeafCommand("date", canonicalLeafOptions{
	RenderHuman: renderDate,
})

func renderDate(_ *cobra.Command, result commandexec.Result) error {
	data, err := commandResultData[readsvc.DatePayload](result)
	if err != nil {
		return handleErrorMsg(ErrInternal, err.Error(), "")
	}
	display := ui.NewDisplayContext()
	fmt.Printf("%s %s\n\n", ui.SectionHeader(data.Date), ui.Hint(fmt.Sprintf("(%s)", data.DayOfWeek)))

	fmt.Println(ui.Divider("Daily Note", display.TermWidth))
	if data.DailyNote != nil {
		fmt.Printf("%s\n\n", ui.Bullet(ui.FilePath(data.DailyNote.FilePath)))
	} else {
		fmt.Printf("%s\n\n", ui.Bullet(ui.Hint(fmt.Sprintf("(not created yet - use 'rvn daily %s' to create)", data.Date))))
	}

	byField := make(map[string][]readsvc.DateAssociation)
	for _, item := range data.Items {
		byField[item.FieldName] = append(byField[item.FieldName], item)
	}

	for fieldName, fieldItems := range byField {
		prettyField := fieldName
		if prettyField != "" {
			prettyField = strings.ToUpper(prettyField[:1]) + prettyField[1:]
		}
		label := fmt.Sprintf("%s: %s (%d)", prettyField, data.Date, len(fieldItems))
		fmt.Println(ui.Divider(label, display.TermWidth))
		for _, item := range fieldItems {
			if item.SourceType == "trait" {
				if item.Trait != nil {
					valueStr := ""
					if idx := item.Trait.IndexValueString(); idx != nil {
						valueStr = *idx
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

	if len(data.Backlinks) > 0 {
		fmt.Println(ui.Divider(fmt.Sprintf("References (%d)", len(data.Backlinks)), display.TermWidth))
		for _, bl := range data.Backlinks {
			location := bl.FilePath
			if bl.Line != nil {
				location = fmt.Sprintf("%s:%d", bl.FilePath, *bl.Line)
			}
			fmt.Println(ui.Bullet(ui.Hint(location)))
		}
	}

	return nil
}

func init() {
	rootCmd.AddCommand(dateCmd)
}
