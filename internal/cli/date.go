package cli

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/spf13/cobra"
)

var dateCmd = &cobra.Command{
	Use:   "date [date]",
	Short: "Show everything related to a date",
	Long: `Shows all objects and traits associated with a specific date.

This includes:
- The daily note for that date (if exists)
- Tasks due on that date
- Events on that date
- Any object with a date field matching that date
- References to that date

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

		fmt.Printf("# %s (%s)\n\n", dateStr, dayOfWeek)

		// Check for daily note
		dailyNoteID := vaultCfg.DailyNoteID(dateStr)
		dailyNote, err := db.GetObject(dailyNoteID)
		if err != nil {
			return fmt.Errorf("failed to query daily note: %w", err)
		}

		if dailyNote != nil {
			fmt.Printf("## Daily Note\n")
			fmt.Printf("  %s\n\n", dailyNote.FilePath)
		} else {
			fmt.Printf("## Daily Note\n")
			fmt.Printf("  (not created yet - use 'rvn daily %s' to create)\n\n", dateStr)
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
			fmt.Printf("## %s: %s (%d)\n", strings.Title(fieldName), dateStr, len(fieldItems))
			for _, item := range fieldItems {
			if item.SourceType == "trait" {
				// Get trait content
				trait, err := db.GetTrait(item.SourceID)
				if err == nil && trait != nil {
					valueStr := ""
					if trait.Value != nil && *trait.Value != "" {
						valueStr = fmt.Sprintf(" (%s)", *trait.Value)
					}
					fmt.Printf("  @%s%s %s\n", trait.TraitType, valueStr, trait.Content)
					fmt.Printf("    %s\n", trait.FilePath)
				}
			} else {
					// Object
					obj, err := db.GetObject(item.SourceID)
					if err == nil && obj != nil {
						fmt.Printf("  %s (%s)\n", item.SourceID, obj.Type)
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
			fmt.Printf("## References (%d)\n", len(backlinks))
			for _, bl := range backlinks {
				lineInfo := ""
				if bl.Line != nil {
					lineInfo = fmt.Sprintf(":%d", *bl.Line)
				}
				fmt.Printf("  %s%s\n", bl.FilePath, lineInfo)
			}
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(dateCmd)
}
