package commandimpl

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

const indexUpdateFailedWarningCode = codes.WarnIndexUpdateFailed

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

// missingRefEnvelope detects references in the given files whose targets do not
// exist yet and returns success-envelope data fields plus REF_NOT_FOUND
// warnings. Writes remain permissive: this only annotates a successful response
// so callers can surface the missing target (interactively in the CLI, or via
// the warning/data for agents). Detection failures are non-fatal and produce no
// annotations.
func missingRefEnvelope(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, relPaths ...string) (map[string]interface{}, []commandexec.Warning) {
	refs, err := checksvc.DetectMissingRefs(vaultPath, vaultCfg, sch, relPaths...)
	if err != nil || len(refs) == 0 {
		return nil, nil
	}
	sort.Slice(refs, func(i, j int) bool {
		return refs[i].TargetPath < refs[j].TargetPath
	})

	warnings := make([]commandexec.Warning, 0, len(refs))
	for _, ref := range refs {
		warnings = append(warnings, missingRefWarning(ref))
	}
	data := map[string]interface{}{
		"missing_refs":      len(refs),
		"missing_ref_items": refs,
	}
	return data, warnings
}

func missingRefWarning(ref *check.MissingRef) commandexec.Warning {
	warning := commandexec.Warning{
		Code:    codes.WarnRefNotFound,
		Message: fmt.Sprintf("Reference [[%s]] does not exist yet", ref.TargetPath),
		Ref:     "Run 'rvn check create-missing' to create missing referenced pages",
	}
	if ref.InferredType != "" {
		warning.SuggestedType = ref.InferredType
		warning.CreateCommand = fmt.Sprintf("rvn new %s %q", ref.InferredType, ref.TargetPath)
	} else {
		warning.CreateCommand = "rvn check create-missing"
	}
	return warning
}

// mergeDataFields copies src entries into dst, allocating dst when needed.
func mergeDataFields(dst map[string]interface{}, src map[string]interface{}) map[string]interface{} {
	if len(src) == 0 {
		return dst
	}
	if dst == nil {
		dst = make(map[string]interface{}, len(src))
	}
	for key, value := range src {
		dst[key] = value
	}
	return dst
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
