package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	interactiveStdinIsTerminal  = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
	interactiveStdoutIsTerminal = func() bool { return term.IsTerminal(os.Stdout.Fd()) }
	ravenRunPicker              = picker.Run
)

func canUseRavenInteractive() bool {
	if isJSONOutput() {
		return false
	}
	return canUseInteractiveTerminal()
}

func canUseInteractiveTerminal() bool {
	return interactiveStdinIsTerminal() && interactiveStdoutIsTerminal()
}

func pickVaultFile(vaultPath string, vaultCfg *config.VaultConfig, prompt, title string) (string, bool, error) {
	paths, err := indexedVaultFilePaths(vaultPath, vaultCfg)
	if err != nil {
		return "", false, err
	}
	if len(paths) == 0 {
		return "", false, fmt.Errorf("no indexed files available (run 'rvn reindex')")
	}

	items := make([]picker.Item, 0, len(paths))
	for _, relPath := range paths {
		items = append(items, picker.Item{
			ID:         relPath,
			Label:      relPath,
			Location:   relPath,
			SearchText: relPath,
			FilePath:   relPath,
		})
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:  title,
		Prompt: strings.TrimSuffix(prompt, "> "),
	})
	if err != nil || !ok {
		return "", ok, err
	}
	return strings.TrimSpace(selected.Item.ID), true, nil
}

func prepareInteractiveReferenceArgs(args []string, commandName, argName, prompt, header string) ([]string, bool, error) {
	if len(args) > 0 {
		return args, false, nil
	}

	vaultPath := getVaultPath()
	if canUseRavenInteractive() {
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		selectedPath, selected, err := pickVaultFile(vaultPath, vaultCfg, prompt, header)
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed files")
		}
		if !selected {
			return nil, true, nil
		}
		return []string{selectedPath}, false, nil
	}

	usage := fmt.Sprintf("rvn %s <%s>", commandName, argName)
	err := handleErrorMsg(
		ErrMissingArgument,
		fmt.Sprintf("specify a %s", argName),
		interactivePickerMissingArgSuggestion(commandName, usage),
	)
	return nil, err == nil, err
}

func pickAmbiguousReference(reference string, matches []string, matchSources map[string]string, prompt string) (string, bool, error) {
	items := make([]picker.Item, 0, len(matches))
	for _, match := range matches {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		source := strings.TrimSpace(matchSources[match])
		detail := ""
		if source != "" {
			detail = "matched via " + source
		}
		items = append(items, picker.Item{
			ID:         match,
			Label:      match,
			Detail:     detail,
			Columns:    []string{match, source},
			SearchText: browseSearchText(match, source),
		})
	}
	if len(items) == 0 {
		return "", false, nil
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:   fmt.Sprintf("Reference %q is ambiguous", reference),
		Prompt:  strings.TrimSuffix(prompt, "> "),
		Headers: []string{"#", "target", "matched via"},
		Columns: ui.BacklinksLayout(),
	})
	if err != nil || !ok {
		return "", ok, err
	}

	target := strings.TrimSpace(selected.Item.ID)
	if target == "" {
		return "", false, nil
	}
	return target, true, nil
}

func indexedVaultFilePaths(vaultPath string, vaultCfg *config.VaultConfig) ([]string, error) {
	db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	paths, err := db.AllIndexedFilePaths()
	if err != nil {
		return nil, err
	}
	sort.Strings(paths)

	out := make([]string, 0, len(paths))
	for _, relPath := range paths {
		if _, err := os.Stat(filepath.Join(vaultPath, relPath)); err == nil {
			out = append(out, relPath)
		}
	}
	return out, nil
}

func interactivePickerMissingArgSuggestion(commandName, usage string) string {
	if canUseInteractiveTerminal() {
		return fmt.Sprintf("Run '%s'", usage)
	}
	return fmt.Sprintf("Run '%s' or use bare 'rvn %s' from an interactive terminal", usage, commandName)
}
