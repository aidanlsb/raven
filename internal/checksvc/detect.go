package checksvc

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// DetectMissingRefs returns the page-style missing references found in the given
// files. It is scoped to those files (it does not walk the whole vault, unlike
// Run) and reuses the same validator/resolver the full check uses, so the
// resulting *check.MissingRef items carry the same type inference: certain from
// typed ref fields, inferred from path matching default_path, or unknown.
//
// Ambiguous references, stale fragments, and missing assets are intentionally
// not reported here, matching trackMissingRef semantics in the check validator.
//
// Detection requires the index to resolve reference targets. If the index is
// unavailable, detection is skipped (returns nil) rather than reporting false
// positives for targets that exist on disk but are not yet indexed.
func DetectMissingRefs(vaultPath string, vaultCfg *config.VaultConfig, sch *schema.Schema, relPaths ...string) ([]*check.MissingRef, error) {
	if vaultCfg == nil || sch == nil || len(relPaths) == 0 {
		return nil, nil
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return nil, nil
	}
	defer db.Close()

	// index.Open creates an empty database if none exists. With an empty index no
	// targets can resolve, so every reference would look "missing". Skip detection
	// in that case rather than report false positives for an unindexed vault.
	if objectIDs, idErr := db.AllObjectIDs(); idErr != nil || len(objectIDs) == 0 {
		return nil, nil
	}

	aliases, _ := db.AllAliases()
	canonicalResolver, _ := db.Resolver(index.ResolverOptions{
		DailyDirectory: vaultCfg.GetDailyDirectory(),
		Schema:         sch,
	})
	if canonicalResolver == nil {
		return nil, nil
	}

	validator := check.NewValidatorWithTypesAliasesAndResolver(sch, nil, aliases, canonicalResolver)
	validator.SetDailyDirectoryForInference(vaultCfg.GetDailyDirectory())
	if vaultCfg.HasDirectoriesConfig() {
		validator.SetDirectoryRoots(vaultCfg.GetObjectsRoot(), vaultCfg.GetPagesRoot())
	}

	parseOpts := &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}

	seen := make(map[string]struct{}, len(relPaths))
	for _, relPath := range relPaths {
		relPath = strings.TrimSpace(relPath)
		if relPath == "" {
			continue
		}
		absPath := filepath.Join(vaultPath, filepath.FromSlash(relPath))
		if _, ok := seen[absPath]; ok {
			continue
		}
		seen[absPath] = struct{}{}

		content, readErr := os.ReadFile(absPath)
		if readErr != nil {
			continue
		}
		doc, parseErr := parser.ParseDocumentWithOptions(string(content), absPath, vaultPath, parseOpts)
		if parseErr != nil {
			continue
		}
		validator.ValidateDocument(doc)
	}

	return validator.MissingRefs(), nil
}
