package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
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

		// Load vault config for daily directory
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load vault config: %w", err)
		}

		// Parse date argument
		var dateArg string
		if len(args) > 0 {
			dateArg = args[0]
		}
		targetDate, err := vault.ParseDateArg(dateArg)
		if err != nil {
			return err
		}

		dateStr := vault.FormatDateISO(targetDate)
		dayOfWeek := targetDate.Format("Monday")

		db, err := index.Open(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to open database: %w", err)
		}
		defer db.Close()

		display := ui.NewDisplayContext()
		fmt.Printf("%s %s\n\n", ui.SectionHeader(dateStr), ui.Hint(fmt.Sprintf("(%s)", dayOfWeek)))

		// Check for daily note
		dailyNoteID := vaultCfg.DailyNoteID(dateStr)
		dailyNote, err := db.GetObject(dailyNoteID)
		if err != nil {
			return fmt.Errorf("failed to query daily note: %w", err)
		}

		fmt.Println(ui.Divider("Daily Note", display.TermWidth))
		if dailyNote != nil {
			fmt.Printf("%s\n\n", ui.Bullet(ui.FilePath(dailyNote.FilePath)))
		} else {
			fmt.Printf("%s\n\n", ui.Bullet(ui.Hint(fmt.Sprintf("(not created yet - use 'rvn daily %s' to create)", dateStr))))
		}

		// Query date index for this date
		items, err := db.QueryDateIndex(dateStr)
		if err != nil {
			return fmt.Errorf("failed to query date index: %w", err)
		}

		// Group by field name
		byField := make(map[string][]index.DateIndexResult)
		for _, item := range items {
			byField[item.FieldName] = append(byField[item.FieldName], item)
		}

		// Display grouped results
		for fieldName, fieldItems := range byField {
			prettyField := fieldName
			if prettyField != "" {
				prettyField = strings.ToUpper(prettyField[:1]) + prettyField[1:]
			}
			label := fmt.Sprintf("%s: %s (%d)", prettyField, dateStr, len(fieldItems))
			fmt.Println(ui.Divider(label, display.TermWidth))
			for _, item := range fieldItems {
				if item.SourceType == "trait" {
					// Get trait content
					trait, err := db.GetTrait(item.SourceID)
					if err == nil && trait != nil {
						valueStr := ""
						if trait.Value != nil && *trait.Value != "" {
							valueStr = *trait.Value
						}
						line := fmt.Sprintf("%s %s", ui.Trait(trait.TraitType, valueStr), trait.Content)
						fmt.Println(ui.Bullet(line))
						fmt.Println(ui.Indent(2, ui.Hint(trait.FilePath)))
					}
				} else {
					// Object
					obj, err := db.GetObject(item.SourceID)
					if err == nil && obj != nil {
						meta := ""
						if obj.Type != "" {
							meta = ui.Hint(fmt.Sprintf("(%s)", obj.Type))
						}
						fmt.Println(ui.Bullet(strings.TrimSpace(fmt.Sprintf("%s %s", item.SourceID, meta))))
					}
				}
			}
			fmt.Println()
		}

		// Query for references to this date
		backlinks, err := db.Backlinks(dateStr)
		if err != nil {
			return fmt.Errorf("failed to query backlinks: %w", err)
		}

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

func init() {
	rootCmd.AddCommand(dateCmd)
}
