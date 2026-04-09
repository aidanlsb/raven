package commandimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

const indexUpdateFailedWarningCode = "INDEX_UPDATE_FAILED"

const indexUpdateFailedWarningRef = "The write succeeded, but the derived index may be stale. Run 'rvn reindex' to refresh it."

func buildParseOptions(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

func autoReindexWarnings(vaultPath string, vaultCfg *config.VaultConfig, filePaths ...string) []commandexec.Warning {
	if vaultCfg == nil || !vaultCfg.IsAutoReindexEnabled() {
		return nil
	}

	seen := make(map[string]struct{}, len(filePaths))
	warnings := make([]commandexec.Warning, 0)
	for _, filePath := range filePaths {
		filePath = strings.TrimSpace(filePath)
		if filePath == "" {
			continue
		}
		filePath = filepath.Clean(filePath)
		if _, ok := seen[filePath]; ok {
			continue
		}
		seen[filePath] = struct{}{}
		if warning, ok := autoReindexWarning(vaultPath, filePath, vaultCfg); ok {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func autoReindexWarning(vaultPath, filePath string, vaultCfg *config.VaultConfig) (commandexec.Warning, bool) {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to load schema", err), true
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to read file", err), true
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, buildParseOptions(vaultCfg))
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to parse file", err), true
	}

	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to open index database", err), true
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	if err := db.IndexDocumentWithMtime(doc, sch, mtime); err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to update index", err), true
	}
	return commandexec.Warning{}, false
}

func indexUpdateWarning(vaultPath, filePath, prefix string, err error) commandexec.Warning {
	displayPath := filepath.ToSlash(filepath.Clean(filePath))
	if relPath, relErr := filepath.Rel(vaultPath, filePath); relErr == nil && !strings.HasPrefix(relPath, "..") {
		displayPath = filepath.ToSlash(relPath)
	}
	return commandexec.Warning{
		Code:    indexUpdateFailedWarningCode,
		Message: fmt.Sprintf("auto-reindex failed for %s: %s: %v", displayPath, prefix, err),
		Ref:     indexUpdateFailedWarningRef,
	}
}

func appendCommandWarnings(groups ...[]commandexec.Warning) []commandexec.Warning {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}
	combined := make([]commandexec.Warning, 0, total)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}
