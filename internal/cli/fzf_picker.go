package cli

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	fzfLookPath         = exec.LookPath
	fzfStdinIsTerminal  = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
	fzfStdoutIsTerminal = func() bool { return term.IsTerminal(os.Stdout.Fd()) }
	ravenRunPicker      = picker.Run
)

type fzfPickerOptions struct {
	Prompt    string
	Header    string
	Delimiter string
	WithNth   string
}

// fzfDefaultAppearance holds Raven's cosmetic defaults for interactive pickers.
// These are injected ahead of the user's FZF_DEFAULT_OPTS so the user can
// override any of them via their own fzf configuration.
const fzfDefaultAppearance = "--layout=reverse --height=80% --border"

func hasFZFInstalled() bool {
	_, err := fzfLookPath("fzf")
	return err == nil
}

// fzfEnv returns the child environment for fzf with Raven's cosmetic defaults
// prepended to FZF_DEFAULT_OPTS. fzf parses FZF_DEFAULT_OPTS left-to-right with
// later options winning, so the user's existing FZF_DEFAULT_OPTS (placed after
// the defaults) overrides any of Raven's defaults it conflicts with.
func fzfEnv() []string {
	merged := fzfDefaultAppearance
	if existing := strings.TrimSpace(os.Getenv("FZF_DEFAULT_OPTS")); existing != "" {
		merged += " " + existing
	}

	base := os.Environ()
	env := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if strings.HasPrefix(kv, "FZF_DEFAULT_OPTS=") {
			continue
		}
		env = append(env, kv)
	}
	return append(env, "FZF_DEFAULT_OPTS="+merged)
}

func canUseFZFInteractive() bool {
	if isJSONOutput() {
		return false
	}
	if !canUseInteractiveTerminal() {
		return false
	}
	return hasFZFInstalled()
}

func canUseRavenInteractive() bool {
	if isJSONOutput() {
		return false
	}
	return canUseInteractiveTerminal()
}

func canUseInteractiveTerminal() bool {
	return fzfStdinIsTerminal() && fzfStdoutIsTerminal()
}

func runFZFPicker(lines []string, opts fzfPickerOptions) (string, bool, error) {
	if len(lines) == 0 {
		return "", false, nil
	}

	// Only behavioral flags that define the picker contract are passed as
	// command-line args. fzf gives command-line args precedence over
	// FZF_DEFAULT_OPTS, so these always take effect. Cosmetic defaults are
	// handled via FZF_DEFAULT_OPTS (see fzfEnv) so users can override them.
	args := []string{
		"--select-1",
		"--exit-0",
	}
	if strings.TrimSpace(opts.Prompt) != "" {
		args = append(args, "--prompt", opts.Prompt)
	}
	if strings.TrimSpace(opts.Header) != "" {
		args = append(args, "--header", opts.Header)
	}
	if strings.TrimSpace(opts.Delimiter) != "" {
		args = append(args, "--delimiter", opts.Delimiter)
	}
	if strings.TrimSpace(opts.WithNth) != "" {
		args = append(args, "--with-nth", opts.WithNth)
	}

	cmd := exec.Command("fzf", args...)
	cmd.Stdin = strings.NewReader(strings.Join(lines, "\n") + "\n")
	cmd.Env = fzfEnv()

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			if code := exitErr.ExitCode(); code == 1 || code == 130 {
				return "", false, nil
			}
		}
		return "", false, fmt.Errorf("run fzf selector: %w", err)
	}

	selection := strings.TrimSpace(stdout.String())
	if selection == "" {
		return "", false, nil
	}
	return selection, true, nil
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
