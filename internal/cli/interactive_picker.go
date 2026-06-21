package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/picker"
	"github.com/aidanlsb/raven/internal/ui"
)

var (
	interactiveStdinIsTerminal  = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
	interactiveStdoutIsTerminal = func() bool { return term.IsTerminal(os.Stdout.Fd()) }
	ravenRunPicker              = picker.Run
)

type interactiveReferencePickerOptions struct {
	IncludeAssets bool
}

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
		items = append(items, fileSelectionItem(relPath))
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

func pickReferenceTarget(vaultPath string, vaultCfg *config.VaultConfig, prompt, title string, opts interactiveReferencePickerOptions) (string, bool, error) {
	items, err := indexedReferenceTargetItems(vaultPath, vaultCfg, opts)
	if err != nil {
		return "", false, err
	}
	if len(items) == 0 {
		return "", false, fmt.Errorf("no indexed references available (run 'rvn reindex')")
	}

	selected, ok, err := ravenRunPicker(items, picker.Options{
		Title:   title,
		Prompt:  strings.TrimSuffix(prompt, "> "),
		Headers: []string{"#", "reference", "kind", "location"},
		Columns: ui.SearchLayout(),
		Preview: vaultFilePreview(vaultPath),
	})
	if err != nil || !ok {
		return "", ok, err
	}
	return strings.TrimSpace(selected.Item.ID), true, nil
}

func prepareInteractiveReferenceArgs(args []string, commandName, argName, prompt, header string, opts interactiveReferencePickerOptions) ([]string, bool, error) {
	if len(args) > 0 {
		return args, false, nil
	}

	vaultPath := getVaultPath()
	if canUseRavenInteractive() {
		vaultCfg, err := loadVaultConfigSafe(vaultPath)
		if err != nil {
			return nil, false, handleError(ErrConfigInvalid, err, "Fix raven.yaml and try again")
		}
		selectedRef, selected, err := pickReferenceTarget(vaultPath, vaultCfg, prompt, header, opts)
		if err != nil {
			return nil, false, handleError(ErrInternal, err, "Run 'rvn reindex' to refresh indexed references")
		}
		if !selected {
			return nil, true, nil
		}
		return []string{selectedRef}, false, nil
	}

	usage := fmt.Sprintf("rvn %s <%s>", commandName, argName)
	err := handleErrorMsg(
		ErrMissingArgument,
		fmt.Sprintf("specify a %s", argName),
		interactivePickerMissingArgSuggestion(commandName, usage),
	)
	return nil, err == nil, err
}

func indexedReferenceTargetItems(vaultPath string, vaultCfg *config.VaultConfig, opts interactiveReferencePickerOptions) ([]picker.Item, error) {
	db, err := openDatabaseWithConfig(vaultPath, vaultCfg)
	if err != nil {
		return nil, err
	}
	defer db.Close()

	objects, err := db.AllObjects()
	if err != nil {
		return nil, err
	}
	sections, err := db.AllSections()
	if err != nil {
		return nil, err
	}
	var assets []model.Asset
	if opts.IncludeAssets {
		assets, err = db.QueryAssets()
		if err != nil {
			return nil, err
		}
	}

	items := make([]picker.Item, 0, len(objects)+len(sections)+len(assets))
	for _, obj := range objects {
		if !indexedReferenceFileExists(vaultPath, obj.FilePath) {
			continue
		}
		items = append(items, pickerItemForObjectReference(obj))
	}
	for _, section := range sections {
		if !indexedReferenceFileExists(vaultPath, section.FilePath) {
			continue
		}
		items = append(items, pickerItemForSectionReference(section))
	}
	for _, asset := range assets {
		if !indexedReferenceFileExists(vaultPath, asset.FilePath) {
			continue
		}
		items = append(items, pickerItemForAssetReference(asset))
	}

	sort.SliceStable(items, func(i, j int) bool {
		if items[i].FilePath != items[j].FilePath {
			return items[i].FilePath < items[j].FilePath
		}
		if items[i].Line != items[j].Line {
			return items[i].Line < items[j].Line
		}
		return items[i].ID < items[j].ID
	})
	return items, nil
}

func pickerItemForObjectReference(obj model.Object) picker.Item {
	location := fmt.Sprintf("%s:%d", obj.FilePath, obj.LineStart)
	kind := obj.Type
	if kind == "" {
		kind = "object"
	}
	return picker.Item{
		ID:       obj.ID,
		Label:    obj.ID,
		Detail:   kind,
		Location: location,
		Columns:  []string{obj.ID, kind, location},
		SearchText: browseSearchText(
			obj.ID,
			fileNameWithoutMarkdown(obj.FilePath),
			obj.Type,
			objectReferenceFieldSearchText(obj),
			obj.FilePath,
			location,
		),
		FilePath: obj.FilePath,
		Line:     obj.LineStart,
	}
}

func pickerItemForSectionReference(section model.Section) picker.Item {
	location := fmt.Sprintf("%s:%d", section.FilePath, section.LineStart)
	kind := fmt.Sprintf("section h%d #%s", section.Level, section.Slug)
	parentSectionID := ""
	if section.ParentSectionID != nil {
		parentSectionID = *section.ParentSectionID
	}
	return picker.Item{
		ID:       section.ID,
		Label:    section.ID,
		Detail:   kind,
		Location: location,
		Columns:  []string{section.ID, kind, location},
		SearchText: browseSearchText(
			section.ID,
			section.Title,
			section.Slug,
			kind,
			section.FileObjectID,
			parentSectionID,
			section.FilePath,
			location,
		),
		FilePath: section.FilePath,
		Line:     section.LineStart,
	}
}

func pickerItemForAssetReference(asset model.Asset) picker.Item {
	location := asset.FilePath
	kind := "asset"
	if asset.MediaType != "" {
		kind += " " + asset.MediaType
	}
	return picker.Item{
		ID:       asset.ID,
		Label:    asset.ID,
		Detail:   kind,
		Location: location,
		Columns:  []string{asset.ID, kind, location},
		SearchText: browseSearchText(
			asset.ID,
			asset.FilePath,
			asset.Filename,
			asset.Extension,
			asset.MediaType,
			location,
		),
		FilePath: asset.FilePath,
	}
}

// fileSelectionItem builds a picker item that selects a vault-relative file
// path. The path is the selection key, the displayed label, and the preview
// target, so choosing the item hands the same path to the caller and the
// preview.
func fileSelectionItem(relPath string) picker.Item {
	return picker.Item{
		ID:         relPath,
		Label:      relPath,
		Location:   relPath,
		SearchText: relPath,
		FilePath:   relPath,
	}
}

func indexedReferenceFileExists(vaultPath, relPath string) bool {
	if relPath == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(vaultPath, relPath))
	return err == nil
}

func objectReferenceFieldSearchText(obj model.Object) string {
	if len(obj.Fields) == 0 {
		return ""
	}
	parts := make([]string, 0, len(obj.Fields))
	for fieldName, value := range obj.Fields {
		valueText := formatFieldValueSimple(value)
		if valueText == "" {
			continue
		}
		parts = append(parts, fieldName+"="+valueText)
	}
	sort.Strings(parts)
	return strings.Join(parts, " ")
}

func fileNameWithoutMarkdown(relPath string) string {
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func pickAmbiguousReference(reference string, matches []string, matchSources map[string]string, prompt string) (string, bool, error) {
	items := make([]picker.Item, 0, len(matches))
	for _, match := range matches {
		match = strings.TrimSpace(match)
		if match == "" {
			continue
		}
		items = append(items, ambiguousReferenceItem(match, strings.TrimSpace(matchSources[match])))
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

// ambiguousReferenceItem builds a picker item for one candidate of an
// ambiguous reference. The candidate string is the selection key; source is
// the match origin shown as display-only detail and table column.
func ambiguousReferenceItem(match, source string) picker.Item {
	detail := ""
	if source != "" {
		detail = "matched via " + source
	}
	return picker.Item{
		ID:         match,
		Label:      match,
		Detail:     detail,
		Columns:    []string{match, source},
		SearchText: browseSearchText(match, source),
	}
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
