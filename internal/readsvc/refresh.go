package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vault"
)

func CheckStaleness(rt *Runtime) (bool, []string, error) {
	if rt == nil || rt.DB == nil {
		return false, nil, fmt.Errorf("runtime with database is required")
	}
	staleness, err := rt.DB.CheckStaleness(rt.VaultPath)
	if err != nil {
		return false, nil, err
	}
	return staleness.IsStale, staleness.StaleFiles, nil
}

func SmartReindex(rt *Runtime) (int, error) {
	if rt == nil || rt.DB == nil {
		return 0, fmt.Errorf("runtime with database is required")
	}

	vaultCfg := rt.VaultCfg
	if vaultCfg == nil {
		loaded, err := config.LoadVaultConfig(rt.VaultPath)
		if err != nil {
			return 0, err
		}
		vaultCfg = loaded
		rt.VaultCfg = loaded
	}
	rt.DB.SetDailyDirectory(vaultCfg.GetDailyDirectory())

	sch := rt.Schema
	if sch == nil {
		loaded, err := schema.Load(rt.VaultPath)
		if err != nil {
			return 0, err
		}
		sch = loaded
	}

	if _, err := rt.DB.RemoveDeletedFiles(rt.VaultPath); err != nil {
		return 0, err
	}

	walkOpts := &vault.WalkOptions{ParseOptions: buildParseOptions(vaultCfg)}
	reindexed := 0
	err := vault.WalkMarkdownFilesWithOptions(rt.VaultPath, walkOpts, func(result vault.WalkResult) error {
		if result.Error != nil {
			return nil //nolint:nilerr // skip files with errors
		}

		indexedMtime, err := rt.DB.GetFileMtime(result.RelativePath)
		if err == nil && indexedMtime > 0 && result.FileMtime <= indexedMtime {
			return nil
		}

		if err := rt.DB.IndexDocumentWithMtime(result.Document, sch, result.FileMtime); err != nil {
			return nil //nolint:nilerr // skip files that fail to index
		}

		reindexed++
		return nil
	})
	if err != nil {
		return 0, err
	}

	return reindexed, nil
}

func buildParseOptions(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil || !vaultCfg.HasDirectoriesConfig() {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}
