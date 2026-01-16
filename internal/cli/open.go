package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/vault"
)

var openCmd = &cobra.Command{
	Use:   "open <reference>",
	Short: "Open a file in your editor",
	Long: `Opens a file in your configured editor.

The reference can be:
  - A short reference: cursor, freya, bifrost
  - A partial path: companies/cursor, people/freya
  - A full path: objects/companies/cursor.md

The editor is determined by (in order):
  1. The 'editor' setting in ~/.config/raven/config.toml
  2. The $EDITOR environment variable

Examples:
  rvn open cursor              # Opens companies/cursor.md
  rvn open companies/cursor    # Opens objects/companies/cursor.md
  rvn open people/freya        # Opens people/freya.md
  rvn open daily/2025-01-09    # Opens a specific daily note`,
	Args: cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		reference := args[0]

		// Load vault config
		vaultCfg := loadVaultConfigSafe(vaultPath)

		// Resolve the reference using unified resolver
		result, err := ResolveReference(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			return handleResolveError(err, reference)
		}

		relPath, _ := filepath.Rel(vaultPath, result.FilePath)

		// JSON output
		if isJSONOutput() {
			cfg := getConfig()
			editor := ""
			if cfg != nil {
				editor = cfg.GetEditor()
			}

			opened := vault.OpenInEditor(cfg, result.FilePath)
			outputSuccess(map[string]interface{}{
				"file":   relPath,
				"opened": opened,
				"editor": editor,
			}, nil)
			return nil
		}

		// Human output - open in editor
		openFileInEditor(result.FilePath, relPath, false)

		return nil
	},
}

// openFileInEditor opens a file in the configured editor and prints appropriate output.
// If skipOpenMessage is true, it won't print "Opening..." (useful when a "Created" message was already shown).
func openFileInEditor(filePath, relPath string, skipOpenMessage bool) {
	cfg := getConfig()
	if vault.OpenInEditor(cfg, filePath) {
		if !skipOpenMessage {
			fmt.Printf("Opening %s\n", relPath)
		}
	} else {
		fmt.Printf("File: %s\n", relPath)
		fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	}
}

func init() {
	rootCmd.AddCommand(openCmd)
}
