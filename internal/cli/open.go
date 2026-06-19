package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/ui"
	"github.com/aidanlsb/raven/internal/vault"
)

var openCmd = newCanonicalLeafCommand("open", canonicalLeafOptions{
	VaultPath:      getVaultPath,
	Args:           validateOpenArgs,
	Prepare:        prepareOpenArgs,
	BuildArgs:      buildOpenArgs,
	HandleErrorCmd: handleCanonicalOpenFailure,
	HandleResult:   handleOpenResult,
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
	if canUseRavenInteractive() {
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		selectedRef, selected, err := pickReferenceTarget(vaultPath, vaultCfg, "open> ", "Select a reference to open", interactiveReferencePickerOptions{IncludeAssets: true})
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed references")
		}
		if !selected {
			return nil, true, nil
		}
		return []string{selectedRef}, false, nil
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
		ids, sectionIDs, err := ReadIDsFromStdin()
		if err != nil {
			return nil, err
		}
		if len(ids) == 0 && len(sectionIDs) == 0 {
			return nil, fmt.Errorf("no object IDs provided on stdin")
		}

		allRefs := make([]string, 0, len(ids)+len(sectionIDs))
		allRefs = append(allRefs, ids...)
		allRefs = append(allRefs, sectionIDs...)
		return map[string]interface{}{
			"stdin":      true,
			"object_ids": allRefs,
		}, nil
	}

	return map[string]interface{}{
		"reference": args[0],
	}, nil
}

func handleCanonicalOpenFailure(cmd *cobra.Command, result commandexec.Result) error {
	return handleAmbiguousReferenceRetry(cmd, result, ambiguousReferenceRetryOptions{
		CommandID: "open",
		ArgKey:    "reference",
		Prompt:    "open/ref> ",
		Render: func(_ *cobra.Command, retryResult commandexec.Result) error {
			return renderSingleOpenResult(canonicalDataMap(retryResult))
		},
	})
}

// openFileInEditor opens a file in the configured editor and prints appropriate output.
// If skipOpenMessage is true, it won't print "Opening..." (useful when a "Created" message was already shown).
func openFileInEditor(filePath, relPath string, skipOpenMessage bool) {
	openFileInEditorAtLine(filePath, relPath, 0, skipOpenMessage)
}

// openFileInEditorAtLine opens a file at a specific line when the configured
// editor supports line targets.
func openFileInEditorAtLine(filePath, relPath string, line int, skipOpenMessage bool) {
	cfg := getConfig()
	if vault.OpenInEditorAtLine(cfg, filePath, line) {
		if !skipOpenMessage {
			if line > 0 {
				fmt.Println(ui.Checkf("Opening %s:%d", ui.FilePath(relPath), line))
			} else {
				fmt.Println(ui.Checkf("Opening %s", ui.FilePath(relPath)))
			}
		}
	} else {
		fmt.Println(ui.SectionHeader("File"))
		if line > 0 {
			fmt.Println(ui.Bullet(fmt.Sprintf("%s:%d", ui.FilePath(relPath), line)))
		} else {
			fmt.Println(ui.Bullet(ui.FilePath(relPath)))
		}
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
	line := intFromAny(data["line_start"])
	if boolValue(data["opened"]) {
		if line > 0 {
			fmt.Println(ui.Checkf("Opening %s:%d", ui.FilePath(file), line))
		} else {
			fmt.Println(ui.Checkf("Opening %s", ui.FilePath(file)))
		}
		return nil
	}
	return renderSingleOpenResult(data)
}

func ambiguousReferenceDetails(raw interface{}) (string, []string, map[string]string) {
	details, ok := raw.(map[string]interface{})
	if !ok {
		return "", nil, nil
	}

	reference, _ := details["reference"].(string)
	matches := stringSliceFromAny(details["matches"])
	matchSources := stringMapFromAny(details["match_sources"])
	return reference, matches, matchSources
}

func renderSingleOpenResult(data map[string]interface{}) error {
	file := stringValue(data["file"])
	line := intFromAny(data["line_start"])
	if boolValue(data["opened"]) {
		if line > 0 {
			fmt.Println(ui.Checkf("Opening %s:%d", ui.FilePath(file), line))
		} else {
			fmt.Println(ui.Checkf("Opening %s", ui.FilePath(file)))
		}
		return nil
	}
	fmt.Println(ui.SectionHeader("File"))
	if line > 0 {
		fmt.Println(ui.Bullet(fmt.Sprintf("%s:%d", ui.FilePath(file), line)))
	} else {
		fmt.Println(ui.Bullet(ui.FilePath(file)))
	}
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
