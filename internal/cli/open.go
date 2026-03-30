package cli

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var openCmd = newCanonicalLeafCommand("open", canonicalLeafOptions{
	VaultPath:    getVaultPath,
	Args:         validateOpenArgs,
	Prepare:      prepareOpenArgs,
	BuildArgs:    buildOpenArgs,
	HandleResult: handleOpenResult,
})

func validateOpenArgs(cmd *cobra.Command, args []string) error {
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
}

func prepareOpenArgs(cmd *cobra.Command, args []string) ([]string, bool, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		return args, false, nil
	}
	if len(args) > 0 {
		return args, false, nil
	}

	vaultPath := getVaultPath()
	if canUseFZFInteractive() {
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		relPath, selected, err := pickVaultFileWithFZF(vaultPath, vaultCfg, "open> ", "Select a file to open (Esc to cancel)")
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
		}
		if !selected {
			return nil, true, nil
		}
		openFileInEditor(filepath.Join(vaultPath, relPath), relPath, false)
		return nil, true, nil
	}

	err := handleErrorMsg(
		ErrMissingArgument,
		"specify a reference",
		interactivePickerMissingArgSuggestion("open", "rvn open <reference>"),
	)
	return nil, err == nil, err
}

func buildOpenArgs(cmd *cobra.Command, args []string) (map[string]interface{}, error) {
	stdin, _ := cmd.Flags().GetBool("stdin")
	if stdin {
		ids, embedded, err := ReadIDsFromStdin()
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 && len(embedded) == 0 {
			return nil, fmt.Errorf("no object IDs provided on stdin")
		}

		allRefs := make([]string, 0, len(ids)+len(embedded))
		allRefs = append(allRefs, ids...)
		allRefs = append(allRefs, embedded...)
		return map[string]interface{}{
			"stdin":      true,
			"object_ids": allRefs,
		}, nil
	}

	return map[string]interface{}{
		"reference": args[0],
	}, nil
}

// openFileInEditor opens a file in the configured editor and prints appropriate output.
// If skipOpenMessage is true, it won't print "Opening..." (useful when a "Created" message was already shown).
func openFileInEditor(filePath, relPath string, skipOpenMessage bool) {
	cfg := getConfig()
	if vault.OpenInEditor(cfg, filePath) {
		if !skipOpenMessage {
			fmt.Println(ui.Checkf("Opening %s", ui.FilePath(relPath)))
		}
	} else {
		fmt.Println(ui.SectionHeader("File"))
		fmt.Println(ui.Bullet(ui.FilePath(relPath)))
		fmt.Println(ui.Hint("Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically."))
	}
}

func handleOpenResult(cmd *cobra.Command, result commandexec.Result) error {
	if isJSONOutput() {
		outputJSON(result)
		return nil
	}

	bulk, _ := cmd.Flags().GetBool("stdin")
	data := canonicalDataMap(result)
	if bulk {
		files := stringSliceFromAny(data["files"])
		errors := stringSliceFromAny(data["errors"])
		if boolValue(data["opened"]) {
			fmt.Println(ui.Checkf("Opening %d file(s)", len(files)))
			for _, p := range files {
				fmt.Println(ui.Bullet(ui.FilePath(p)))
			}
		} else {
			fmt.Println(ui.SectionHeader("Files"))
			for _, p := range files {
				fmt.Println(ui.Bullet(ui.FilePath(p)))
			}
			fmt.Println(ui.Hint("Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically."))
		}
		for _, e := range errors {
			fmt.Println(ui.Warning(e))
		}
		return nil
	}

	file := stringValue(data["file"])
	if boolValue(data["opened"]) {
		fmt.Println(ui.Checkf("Opening %s", ui.FilePath(file)))
		return nil
	}
	fmt.Println(ui.SectionHeader("File"))
	fmt.Println(ui.Bullet(ui.FilePath(file)))
	fmt.Println(ui.Hint("Set 'editor' in ~/.config/raven/config.toml or $EDITOR to open automatically."))
	return nil
}

func init() {
	openCmd.ValidArgsFunction = completeReferenceArgAt(0, referenceCompletionOptions{
		IncludeDynamicDates: true,
		DisableWhenStdin:    true,
		NonTargetDirective:  cobra.ShellCompDirectiveNoFileComp,
	})
	rootCmd.AddCommand(openCmd)
}
