package commandimpl

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/mutation"
	"github.com/aidanlsb/raven/internal/paths"
	"github.com/aidanlsb/raven/internal/vault"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// applyChangeSet coordinates derived post-write work for an applied mutation.
// Markdown files remain the durable source of truth: projection failures are
// returned as warnings and never turn a successful write into a failed command.
func applyChangeSet(rt *vaultruntime.Runtime, changes mutation.ChangeSet) (map[string]interface{}, []commandexec.Warning) {
	if rt == nil || changes.Empty() {
		return nil, nil
	}

	var warnings []commandexec.Warning
	if rt.VaultCfg != nil && rt.VaultCfg.IsAutoReindexEnabled() {
		for _, relPath := range changes.RemovedPaths() {
			if warning, ok := removeIndexPathWarning(rt, relPath); ok {
				warnings = append(warnings, warning)
			}
		}
		for _, relPath := range changes.IndexPaths() {
			if warning, ok := projectIndexPathWarning(rt, relPath); ok {
				warnings = append(warnings, warning)
			}
		}
	}

	missingPaths := make([]string, 0, len(changes.IndexPaths()))
	for _, relPath := range changes.IndexPaths() {
		if paths.HasMDExtension(relPath) {
			missingPaths = append(missingPaths, relPath)
		}
	}
	missingData, missingWarnings := missingRefEnvelope(rt, missingPaths...)
	return missingData, appendCommandWarnings(warnings, missingWarnings)
}

func removeIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (commandexec.Warning, bool) {
	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to open index database", err), true
	}
	if err := rt.DB.RemoveFile(relPath); err != nil {
		return indexUpdateWarning(rt.VaultPath, filepath.Join(rt.VaultPath, filepath.FromSlash(relPath)), "failed to remove file from index", err), true
	}
	return commandexec.Warning{}, false
}

func projectIndexPathWarning(rt *vaultruntime.Runtime, relPath string) (commandexec.Warning, bool) {
	filePath := filepath.Join(rt.VaultPath, filepath.FromSlash(relPath))
	if paths.HasMDExtension(relPath) {
		return autoReindexWarning(rt, filePath)
	}

	if rt.VaultCfg == nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to index asset", fmt.Errorf("vault config is required")), true
	}
	info, err := os.Stat(filePath)
	if err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to read file", err), true
	}
	if err := rt.OpenDB(); err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to open index database", err), true
	}
	asset := vault.BuildAsset(relPath, info, rt.VaultCfg)
	if err := rt.DB.IndexAsset(asset); err != nil {
		return indexUpdateWarning(rt.VaultPath, filePath, "failed to update index", err), true
	}
	return commandexec.Warning{}, false
}
