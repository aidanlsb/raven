package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/ravenscroftj/raven/internal/parser"
	"github.com/ravenscroftj/raven/internal/schema"
	"github.com/spf13/cobra"
)

var newType string

var newCmd = &cobra.Command{
	Use:   "new <title>",
	Short: "Create a new typed note",
	Long: `Creates a new note with the specified type and title.

Examples:
  rvn new --type person "Alice Chen"
  rvn new --type project "Website Redesign"`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		title := args[0]

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		// Check if type exists
		if _, ok := s.Types[newType]; !ok && newType != "page" {
			fmt.Printf("Warning: Type '%s' is not defined in schema.yaml\n", newType)
		}

		// Generate filename from title
		filename := parser.Slugify(title)

		// Validate filename
		if filename == "" || strings.Contains(filename, "..") || strings.ContainsAny(filename, "/\\") {
			return fmt.Errorf("invalid title: cannot generate safe filename")
		}

		// Also check for any remaining special characters
		for _, r := range filename {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
				return fmt.Errorf("invalid title: cannot generate safe filename")
			}
		}

		filePath := filepath.Join(vaultPath, filename+".md")

		// Security: verify path is within vault
		absVault, _ := filepath.Abs(vaultPath)
		absFile, _ := filepath.Abs(filePath)
		if !strings.HasPrefix(absFile, absVault) {
			return fmt.Errorf("cannot create file outside vault")
		}

		// Check if file exists
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Errorf("file already exists: %s", filePath)
		}

		content := fmt.Sprintf(`---
type: %s
---

# %s

`, newType, title)

		if err := os.WriteFile(filePath, []byte(content), 0644); err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		fmt.Printf("Created: %s\n", filePath)

		// Try to open in editor
		editor := getConfig().GetEditor()
		if editor != "" {
			execCmd := exec.Command(editor, filePath)
			execCmd.Start()
		}

		return nil
	},
}

func init() {
	newCmd.Flags().StringVarP(&newType, "type", "t", "page", "Type of note to create")
	rootCmd.AddCommand(newCmd)
}
