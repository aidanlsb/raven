package cli

import (
	"fmt"
	"path/filepath"

	"github.com/ravenscroftj/raven/internal/config"
	"github.com/ravenscroftj/raven/internal/pages"
	"github.com/ravenscroftj/raven/internal/vault"
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
		targetPath := filepath.Join(vaultCfg.DailyDirectory, dateStr)
		dailyPath := filepath.Join(vaultPath, vaultCfg.DailyDirectory, dateStr+".md")

		// Check if daily note already exists
		if !pages.Exists(vaultPath, targetPath) {
			friendlyDate := vault.FormatDateFriendly(targetDate)

			result, err := pages.Create(pages.CreateOptions{
				VaultPath:  vaultPath,
				TypeName:   "date",
				Title:      friendlyDate,
				TargetPath: targetPath,
			})
			if err != nil {
				return fmt.Errorf("failed to create daily note: %w", err)
			}

			fmt.Printf("Created: %s\n", result.RelativePath)
			dailyPath = result.FilePath
		} else {
			relPath, _ := filepath.Rel(vaultPath, dailyPath)
			fmt.Printf("Daily note: %s\n", relPath)
		}

		// Open in editor
		vault.OpenInEditor(getConfig(), dailyPath)

		return nil
	},
}

func init() {
	rootCmd.AddCommand(dailyCmd)
}
