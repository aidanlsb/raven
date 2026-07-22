package commandimpl

import (
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/checksvc"
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const indexUpdateFailedWarningCode = codes.WarnIndexUpdateFailed

const indexUpdateFailedWarningRef = "The write succeeded, but the derived index may be stale. Run 'rvn reindex' to refresh it."

func newRequiredCommandVaultRuntime(vaultPath string, openDB bool) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{OpenDB: openDB, RequireSchema: true})
}

func newConfigCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{})
}

func newConfigOnlyCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{SkipSchema: true})
}

func newSchemaOnlyCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		SkipConfig:    true,
		RequireSchema: true,
	})
}

func newSchemaFirstCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		RequireSchema: true,
		SchemaFirst:   true,
	})
}

func newLazyConfigCommandRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		SkipConfig: true,
		SkipSchema: true,
	})
}

func newVaultConfigCommandRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newLazyConfigCommandRuntime(vaultPath)
}

func newDatabaseCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		OpenDB:     true,
		SkipConfig: true,
		SkipSchema: true,
	})
}

func newCommandVaultRuntime(vaultPath string, opts vaultruntime.Options) (*vaultruntime.Runtime, commandexec.Result) {
	rt, err := vaultruntime.New(strings.TrimSpace(vaultPath), opts)
	if err == nil {
		return rt, commandexec.Result{}
	}
	return nil, mapVaultRuntimeSetupFailure(err)
}

func openCommandRuntimeDB(rt *vaultruntime.Runtime) commandexec.Result {
	if err := rt.OpenDB(); err != nil {
		return mapVaultRuntimeSetupFailure(err)
	}
	return commandexec.Result{}
}

func mapVaultRuntimeSetupFailure(err error) commandexec.Result {
	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) {
		switch setupErr.Stage {
		case vaultruntime.StageConfig:
			return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
		case vaultruntime.StageSchema:
			return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
		case vaultruntime.StageDatabase:
			if failure, ok := mapIndexRebuildRequired(err); ok {
				return failure
			}
			return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
		}
	}

	return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
}

func autoReindexWarnings(rt *vaultruntime.Runtime, filePaths ...string) []commandexec.Warning {
	if rt == nil || rt.VaultCfg == nil || !rt.VaultCfg.IsAutoReindexEnabled() {
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
		if warning, ok := autoReindexWarning(rt, filePath); ok {
			warnings = append(warnings, warning)
		}
	}
	return warnings
}

func autoReindexWarning(rt *vaultruntime.Runtime, filePath string) (commandexec.Warning, bool) {
	vaultPath := rt.VaultPath
	if rt.SchemaLoadErr != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to load schema", rt.SchemaLoadErr), true
	}
	if rt.Schema == nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to load schema", errors.New("schema runtime is required")), true
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to read file", err), true
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, rt.ParseOptions)
	if err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to parse file", err), true
	}

	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(vaultPath, filePath, "failed to open index database", err), true
	}
	if err := rt.DB.IndexDocumentWithMtime(doc, rt.Schema, mtime); err != nil {
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
// exist yet and returns success-envelope data fields plus REF_TARGET_MISSING
// warnings. Writes remain permissive: this only annotates a successful response
// so callers can surface the missing target (interactively in the CLI, or via
// the warning/data for agents). Detection failures are non-fatal and produce no
// annotations.
func missingRefEnvelope(rt *vaultruntime.Runtime, relPaths ...string) (map[string]interface{}, []commandexec.Warning) {
	refs, err := checksvc.DetectMissingRefs(rt, relPaths...)
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
		Code:    codes.WarnRefTargetMissing,
		Message: fmt.Sprintf("Reference [[%s]] does not exist yet", ref.TargetPath),
		Ref:     "Run 'rvn check create-missing' to create missing referenced pages",
	}
	if ref.InferredType != "" {
		title := missingRefTitle(ref.TargetPath)
		warning.SuggestedType = ref.InferredType
		warning.CreateCommand = fmt.Sprintf("rvn new %s %q --path %q --json", ref.InferredType, title, ref.TargetPath)
		warning.CreateInvoke = &commandexec.Invoke{
			Command: "new",
			Args: map[string]interface{}{
				"type":  ref.InferredType,
				"title": title,
				"path":  ref.TargetPath,
			},
		}
	} else {
		warning.CreateCommand = "rvn check create-missing"
		warning.CreateInvoke = &commandexec.Invoke{
			Command: "check create-missing",
			Args:    map[string]interface{}{"confirm": true},
		}
	}
	return warning
}

// missingRefTitle derives a concise display title from a reference target path.
// The missing-ref flow always creates the object at the ref's exact location via
// the invocation's explicit --path argument, so we only need a readable display
// title here and use the final path segment (e.g. "people/ghost" -> "ghost").
func missingRefTitle(targetPath string) string {
	trimmed := strings.TrimSpace(targetPath)
	trimmed = strings.TrimRight(trimmed, "/")
	if trimmed == "" {
		return "new-object"
	}
	base := path.Base(trimmed)
	if base == "" || base == "." || base == "/" {
		return "new-object"
	}
	return base
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
