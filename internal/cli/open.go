package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
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
  rvn open daily/2025-01-09    # Opens a specific daily note`,
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

		// Handle --stdin mode
		if openStdin {
			return runOpenStdin(vaultPath)
		}

		if len(args) == 0 {
			if canUseFZFInteractive() {
				vaultCfg, err := loadVaultConfigSafe(vaultPath)
				if err != nil {
					return handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
				}
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
		result := executeCanonicalCommand("open", vaultPath, map[string]interface{}{
			"reference": reference,
		})
		return handleCanonicalOpenResult(result, false)
	},
}

func runOpenStdin(vaultPath string) error {
	ids, embedded, err := ReadIDsFromStdin()
	if err != nil {
		return err
	}

	if len(ids) == 0 && len(embedded) == 0 {
		return fmt.Errorf("no object IDs provided on stdin")
	}

	allRefs := make([]string, 0, len(ids)+len(embedded))
	allRefs = append(allRefs, ids...)
	allRefs = append(allRefs, embedded...)

	result := executeCanonicalCommand("open", vaultPath, map[string]interface{}{
		"stdin":      true,
		"object_ids": allRefs,
	})
	return handleCanonicalOpenResult(result, true)
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

func handleCanonicalOpenResult(result commandexec.Result, bulk bool) error {
	if !result.OK {
		if isJSONOutput() {
			outputJSON(result)
			return nil
		}
		if result.Error != nil {
			return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
		}
		return handleErrorMsg(ErrInternal, "command execution failed", "")
	}

	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	data := canonicalDataMap(result)
	if bulk {
		files := stringSliceFromAny(data["files"])
		errors := stringSliceFromAny(data["errors"])
		if boolValue(data["opened"]) {
			fmt.Printf("Opening %d file(s)\n", len(files))
			for _, p := range files {
				fmt.Printf("  %s\n", p)
			}
		} else {
			fmt.Println("Files:")
			for _, p := range files {
				fmt.Printf("  %s\n", p)
			}
			fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
		}
		for _, e := range errors {
			fmt.Printf("Warning: %s\n", e)
		}
		return nil
	}

	file := stringValue(data["file"])
	if boolValue(data["opened"]) {
		fmt.Printf("Opening %s\n", file)
		return nil
	}
	fmt.Printf("File: %s\n", file)
	fmt.Println("(Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically)")
	return nil
}

func init() {
	openCmd.Flags().BoolVar(&openStdin, "stdin", false, "Read object IDs from stdin")
	rootCmd.AddCommand(openCmd)
}
