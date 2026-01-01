package cli

import (
	"bufio"
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

The file location is determined by the type's default_path setting in schema.yaml.
If no default_path is set, the file is created in the vault root.

Required fields (as defined in schema.yaml) will be prompted for interactively.

Examples:
  rvn new --type person "Alice Chen"    # Creates people/alice-chen.md, prompts for required fields
  rvn new --type project "Website"      # Creates projects/website.md
  rvn new "Quick Note"                  # Creates quick-note.md in vault root (type: page)`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		title := args[0]

		// Load schema
		s, err := schema.Load(vaultPath)
		if err != nil {
			return fmt.Errorf("failed to load schema: %w", err)
		}

		// Check if type exists and get definition
		typeDef, typeExists := s.Types[newType]
		if !typeExists && newType != "page" && newType != "section" {
			return fmt.Errorf("type '%s' is not defined in schema.yaml", newType)
		}

		// Get default_path
		var defaultPath string
		if typeDef != nil && typeDef.DefaultPath != "" {
			defaultPath = typeDef.DefaultPath
		}

		// Collect required field values
		fieldValues := make(map[string]string)
		if typeDef != nil {
			reader := bufio.NewReader(os.Stdin)
			for fieldName, fieldDef := range typeDef.Fields {
				if fieldDef.Required {
					fmt.Printf("%s (required): ", fieldName)
					value, err := reader.ReadString('\n')
					if err != nil {
						return fmt.Errorf("failed to read input: %w", err)
					}
					value = strings.TrimSpace(value)
					if value == "" {
						return fmt.Errorf("required field '%s' cannot be empty", fieldName)
					}
					fieldValues[fieldName] = value
				}
			}
		}

		// Generate filename from title
		filename := parser.Slugify(title)

		// Validate filename
		if filename == "" {
			return fmt.Errorf("invalid title: cannot generate safe filename")
		}
		for _, r := range filename {
			if !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '-' && r != '_' {
				return fmt.Errorf("invalid title: cannot generate safe filename")
			}
		}

		// Build file path
		var filePath string
		if defaultPath != "" {
			// Validate default_path doesn't escape vault
			cleanPath := filepath.Clean(defaultPath)
			if strings.Contains(cleanPath, "..") || filepath.IsAbs(cleanPath) {
				return fmt.Errorf("invalid default_path in schema: %s", defaultPath)
			}
			dirPath := filepath.Join(vaultPath, cleanPath)
			filePath = filepath.Join(dirPath, filename+".md")

			// Create directory if it doesn't exist
			if err := os.MkdirAll(dirPath, 0755); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", cleanPath, err)
			}
		} else {
			filePath = filepath.Join(vaultPath, filename+".md")
		}

		// Security: verify path is within vault
		absVault, _ := filepath.Abs(vaultPath)
		absFile, _ := filepath.Abs(filePath)
		if !strings.HasPrefix(absFile, absVault+string(filepath.Separator)) && absFile != absVault {
			return fmt.Errorf("cannot create file outside vault")
		}

		// Check if file exists
		if _, err := os.Stat(filePath); err == nil {
			return fmt.Errorf("file already exists: %s", filePath)
		}

		// Build frontmatter
		var frontmatter strings.Builder
		frontmatter.WriteString("---\n")
		frontmatter.WriteString(fmt.Sprintf("type: %s\n", newType))
		for fieldName, value := range fieldValues {
			// Handle different value types appropriately
			if strings.ContainsAny(value, ":\n\"'") {
				// Quote values that need it
				frontmatter.WriteString(fmt.Sprintf("%s: \"%s\"\n", fieldName, strings.ReplaceAll(value, "\"", "\\\"")))
			} else {
				frontmatter.WriteString(fmt.Sprintf("%s: %s\n", fieldName, value))
			}
		}
		frontmatter.WriteString("---\n\n")
		frontmatter.WriteString(fmt.Sprintf("# %s\n\n", title))

		if err := os.WriteFile(filePath, []byte(frontmatter.String()), 0644); err != nil {
			return fmt.Errorf("failed to create file: %w", err)
		}

		// Show relative path from vault
		relPath, _ := filepath.Rel(vaultPath, filePath)
		fmt.Printf("Created: %s\n", relPath)

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
