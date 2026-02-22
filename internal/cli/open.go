package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/vault"
)

var openStdin bool

var openCmd = &cobra.Command{
	Use:   "open [reference]",
	Short: "Open a file in your editor",
	Long: `Opens a file in your configured editor.

The reference can be:
  - A short reference: cursor, freya, bifrost
  - A partial path: companies/cursor, people/freya
  - A full path: objects/companies/cursor.md

The editor is determined by (in order):
  1. The 'editor' setting in ~/.config/raven/config.toml
  2. The $EDITOR environment variable

Use --stdin to read object IDs from stdin (one per line) and open them all.
This is useful for piping query results:
  rvn query 'object:project .status==active' --ids | rvn open --stdin

In an interactive terminal with fzf installed, bare 'rvn open' launches
an interactive file picker.

Examples:
  rvn open cursor              # Opens companies/cursor.md
  rvn open companies/cursor    # Opens objects/companies/cursor.md
  rvn open people/freya        # Opens people/freya.md
  rvn open daily/2025-01-09    # Opens a specific daily note
  rvn last 1,3,5 | rvn open --stdin  # Opens selected query results`,
	Args: func(cmd *cobra.Command, args []string) error {
		stdin, _ := cmd.Flags().GetBool("stdin")
		if stdin {
			if len(args) > 0 {
				return fmt.Errorf("cannot specify reference when using --stdin")
			}
			return nil
		}
		if len(args) > 1 {
			return fmt.Errorf("accepts at most 1 argument")
		}
		return nil
	},
	ValidArgsFunction: completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	}),
	RunE: func(cmd *cobra.Command, args []string) error {
		vaultPath := getVaultPath()
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}

		// Handle --stdin mode
		if openStdin {
			return runOpenStdin(vaultPath, vaultCfg)
		}

		if len(args) == 0 {
			if canUseFZFInteractive() {
				relPath, selected, err := pickVaultFileWithFZF(vaultPath, vaultCfg, "open> ", "Select a file to open (Esc to cancel)")
				if err != nil {
					return handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
				}
				if !selected {
					return nil
				}
				openFileInEditor(filepath.Join(vaultPath, relPath), relPath, false)
				return nil
			}

			return handleErrorMsg(
				ErrMissingArgument,
				"specify a reference",
				interactivePickerMissingArgSuggestion("open", "rvn open <reference>"),
			)
		}

		reference := args[0]

		// Resolve the reference using unified resolver, then dynamic date keywords.
		result, err := resolveReferenceWithDynamicDates(reference, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		}, false)
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

func runOpenStdin(vaultPath string, vaultCfg *config.VaultConfig) error {
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return err
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return fmt.Errorf("no object IDs provided on stdin")
	}

	var filePaths []string
	var relPaths []string
	var errors []string

	// Warn about skipped embedded IDs
	for _, id := range embedded {
		errors = append(errors, fmt.Sprintf("%s: embedded objects not supported", id))
	}

	// Resolve each ID to a file path
	for _, id := range ids {
		result, err := ResolveReference(id, ResolveOptions{
			VaultPath:   vaultPath,
			VaultConfig: vaultCfg,
		})
		if err != nil {
			errors = append(errors, fmt.Sprintf("%s: %v", id, err))
			continue
		}
		filePaths = append(filePaths, result.FilePath)
		relPath, _ := filepath.Rel(vaultPath, result.FilePath)
		relPaths = append(relPaths, relPath)
	}

	if len(filePaths) == 0 {
		if len(errors) > 0 {
			return fmt.Errorf("no files to open: %s", errors[0])
		}
		return fmt.Errorf("no files to open")
	}

	cfg := getConfig()

	// JSON output
	if isJSONOutput() {
		editor := ""
		if cfg != nil {
			editor = cfg.GetEditor()
		}

		opened := vault.OpenFilesInEditor(cfg, filePaths)
		outputSuccess(map[string]interface{}{
			"files":  relPaths,
			"opened": opened,
			"editor": editor,
			"errors": errors,
		}, nil)
		return nil
	}

	// Human output
	if vault.OpenFilesInEditor(cfg, filePaths) {
		fmt.Printf("Opening %d file(s)\n", len(filePaths))
		for _, p := range relPaths {
			fmt.Printf("  %s\n", p)
		}
	} else {
		fmt.Println("Files:")
		for _, p := range relPaths {
			fmt.Printf("  %s\n", p)
		}
		fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	}

	// Print any errors
	for _, e := range errors {
		fmt.Printf("Warning: %s\n", e)
	}

	return nil
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
	openCmd.Flags().BoolVar(&openStdin, "stdin", false, "Read object IDs from stdin")
	rootCmd.AddCommand(openCmd)
}
