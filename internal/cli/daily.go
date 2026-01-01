package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/spf13/cobra"
)

var dailyCmd = &cobra.Command{
	Use:   "daily [date]",
	Short: "Open or create a daily note",
	Long: `Creates a daily note if it doesn't exist, then opens it in your editor.

If no date is provided, defaults to today.

Examples:
  rvn daily              # Today's note
  rvn daily yesterday    # Yesterday's note
  rvn daily tomorrow     # Tomorrow's note  
  rvn daily 2025-02-01   # Specific date`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		// Load vault config for daily directory
		vaultCfg, err := config.LoadVaultConfig(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load vault config: %w", err)
		}

		// Determine which date to use
		var targetDate time.Time
		if len(args) > 0 {
			dateArg := strings.ToLower(strings.TrimSpace(args[0]))
			switch dateArg {
			case "today":
				targetDate = time.Now()
			case "yesterday":
				targetDate = time.Now().AddDate(0, 0, -1)
			case "tomorrow":
				targetDate = time.Now().AddDate(0, 0, 1)
			default:
				// Try to parse as YYYY-MM-DD
				parsed, err := time.Parse("2006-01-02", dateArg)
				if err != nil {
					return fmt.Errorf("invalid date format '%s', use YYYY-MM-DD", dateArg)
				}
				targetDate = parsed
			}
		} else {
			targetDate = time.Now()
		}

		dateStr := targetDate.Format("2006-01-02")
		dailyDir := filepath.Join(vaultPath, vaultCfg.DailyDirectory)
		dailyPath := filepath.Join(dailyDir, dateStr+".md")

		if _, err := os.Stat(dailyPath); os.IsNotExist(err) {
			// Create the daily note
			if err := os.MkdirAll(dailyDir, 0755); err != nil {
				return fmt.Errorf("failed to create daily directory: %w", err)
			}

			// Format: "Monday, January 02, 2006"
			friendlyDate := targetDate.Format("Monday, January 2, 2006")

			content := fmt.Sprintf(`---
type: date
---

# %s

`, friendlyDate)

			if err := os.WriteFile(dailyPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create daily note: %w", err)
			}

			relPath, _ := filepath.Rel(vaultPath, dailyPath)
			fmt.Printf("Created: %s\n", relPath)
		} else {
			relPath, _ := filepath.Rel(vaultPath, dailyPath)
			fmt.Printf("Daily note: %s\n", relPath)
		}

		// Try to open in editor
		editor := getConfig().GetEditor()
		if editor != "" {
			execCmd := exec.Command(editor, dailyPath)
			execCmd.Start()
		}

		return nil
	},
}

func init() {
	rootCmd.AddCommand(dailyCmd)
}
