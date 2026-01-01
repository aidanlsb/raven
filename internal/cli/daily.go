package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

var dailyCmd = &cobra.Command{
	Use:   "daily",
	Short: "Open or create today's daily note",
	Long:  `Creates today's daily note if it doesn't exist, then opens it in your editor.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()

		today := time.Now().Format("2006-01-02")
		dailyDir := filepath.Join(vaultPath, "daily")
		dailyPath := filepath.Join(dailyDir, today+".md")

		if _, err := os.Stat(dailyPath); os.IsNotExist(err) {
			// Create the daily note
			if err := os.MkdirAll(dailyDir, 0755); err != nil {
				return fmt.Errorf("failed to create daily directory: %w", err)
			}

			// Format: "Monday, January 02, 2006"
			friendlyDate := time.Now().Format("Monday, January 2, 2006")

			content := fmt.Sprintf(`---
type: daily
date: %s
---

# %s

`, today, friendlyDate)

			if err := os.WriteFile(dailyPath, []byte(content), 0644); err != nil {
				return fmt.Errorf("failed to create daily note: %w", err)
			}

			fmt.Printf("Created: %s\n", dailyPath)
		} else {
			fmt.Printf("Today's note: %s\n", dailyPath)
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
