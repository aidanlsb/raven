package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
)

// workspace holds the resolved vault and a read-side catalog. The catalog is
// rebuilt on save and when another database handle changes resolver-relevant
// index state.
type workspace struct {
	rt *readsvc.Runtime

	catalog     readsvc.CatalogSnapshot
	objectInfos []check.ObjectInfo
}

// isVaultDir reports whether dir looks like a Raven vault root.
func isVaultDir(dir string) bool {
	if dir == "" {
		return false
	}
	for _, marker := range []string{"raven.yaml", "schema.yaml", ".raven"} {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return true
		}
	}
	return false
}

// openWorkspace opens the vault at vaultPath, brings the index up to date,
// and builds the initial caches.
func openWorkspace(vaultPath string) (*workspace, error) {
	absPath, err := filepath.Abs(vaultPath)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve vault path: %w", err)
	}
	if info, err := os.Stat(absPath); err != nil || !info.IsDir() {
		return nil, fmt.Errorf("vault directory not found: %s", absPath)
	}

	rt, err := readsvc.NewRuntime(absPath, readsvc.RuntimeOptions{OpenDB: true})
	if err != nil {
		return nil, fmt.Errorf("failed to open vault %s: %w", absPath, err)
	}
	// The LSP tolerates a broken schema (diagnostics degrade to the built-in
	// schema) but never silently: surface the failure on stderr.
	if rt.SchemaLoadErr != nil {
		fmt.Fprintf(os.Stderr, "rvn lsp: schema load failed, using built-in schema: %v\n", rt.SchemaLoadErr)
	}

	ws := &workspace{rt: rt}
	if report, err := readsvc.SmartReindex(rt); err != nil {
		// A failed reindex leaves stale (but usable) index data; don't abort startup.
		fmt.Fprintf(os.Stderr, "rvn lsp: initial reindex failed: %v\n", err)
	} else if len(report.Failures) > 0 {
		fmt.Fprintf(os.Stderr, "rvn lsp: initial reindex skipped %d file(s)\n", len(report.Failures))
	}
	if err := ws.rebuildCaches(); err != nil {
		rt.Close()
		return nil, err
	}
	return ws, nil
}

func (ws *workspace) close() {
	if ws == nil {
		return
	}
	ws.rt.Close()
}

func (ws *workspace) vaultPath() string {
	return ws.rt.VaultPath
}

func (ws *workspace) vaultConfig() *config.VaultConfig {
	return ws.rt.VaultCfg
}

func (ws *workspace) schema() *schema.Schema {
	if ws.rt.Schema != nil {
		return ws.rt.Schema
	}
	return schema.New()
}

// refresh reloads vault config and schema from disk, incrementally reindexes
// changed files, and rebuilds derived caches. Called after didSave.
func (ws *workspace) refresh() error {
	// Reload config and schema from disk. A reload failure keeps the previous
	// (stale) value so the workspace stays usable, but it is never swallowed:
	// report it on stderr so the degraded state is visible.
	if err := ws.rt.ReloadConfig(); err != nil {
		fmt.Fprintf(os.Stderr, "rvn lsp: config reload failed, keeping previous config: %v\n", err)
	}
	_ = ws.rt.ReloadSchema(false)
	if ws.rt.SchemaLoadErr != nil {
		fmt.Fprintf(os.Stderr, "rvn lsp: schema reload failed, keeping previous schema: %v\n", ws.rt.SchemaLoadErr)
	}

	if report, err := readsvc.SmartReindex(ws.rt); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	} else if len(report.Failures) > 0 {
		fmt.Fprintf(os.Stderr, "rvn lsp: reindex skipped %d file(s)\n", len(report.Failures))
	}
	return ws.rebuildCaches()
}

// ensureCachesFresh reloads derived state after another SQLite handle commits
// resolver-relevant changes. It deliberately avoids a filesystem walk: the
// external writer has already brought the on-disk index up to date.
func (ws *workspace) ensureCachesFresh() error {
	current, err := readsvc.Catalog(ws.rt, readsvc.CatalogOptions{})
	if err != nil {
		return fmt.Errorf("failed to read index generation: %w", err)
	}
	if current.Generation == ws.catalog.Generation {
		return nil
	}
	return ws.rebuildCaches()
}

func (ws *workspace) rebuildCaches() error {
	catalog, err := readsvc.Catalog(ws.rt, readsvc.CatalogOptions{
		Objects:    true,
		Sections:   true,
		Aliases:    true,
		Resolver:   true,
		Consistent: true,
	})
	if err != nil {
		return err
	}
	sort.Slice(catalog.Objects, func(i, j int) bool { return catalog.Objects[i].ID < catalog.Objects[j].ID })

	infos := make([]check.ObjectInfo, 0, len(catalog.Objects))
	for _, obj := range catalog.Objects {
		infos = append(infos, check.ObjectInfo{ID: obj.ID, Type: obj.Type})
	}

	ws.catalog = catalog
	ws.objectInfos = infos
	return nil
}

// newValidator builds a document validator backed by the cached index state.
func (ws *workspace) newValidator() *check.Validator {
	validator := check.NewValidatorWithTypesAliasesAndResolver(
		ws.schema(),
		ws.objectInfos,
		ws.catalog.Aliases,
		ws.catalog.Resolver,
	)
	validator.SetDailyDirectoryForInference(ws.vaultConfig().GetDailyDirectory())
	if ws.vaultConfig().HasDirectoriesConfig() {
		validator.SetDirectoryRoots(ws.vaultConfig().GetObjectsRoot(), ws.vaultConfig().GetPagesRoot())
	}
	return validator
}

// parseBuffer parses in-memory buffer content as the document at absPath.
func (ws *workspace) parseBuffer(content, absPath string) (*parser.ParsedDocument, error) {
	return parser.ParseDocumentWithOptions(content, absPath, ws.vaultPath(), parseopts.FromVaultConfig(ws.vaultConfig()))
}

// relativePath converts an absolute path to a vault-relative slash path.
// Returns "" when the path is outside the vault.
func (ws *workspace) relativePath(absPath string) string {
	rel, err := filepath.Rel(ws.vaultPath(), absPath)
	if err != nil {
		return ""
	}
	rel = filepath.ToSlash(rel)
	if rel == ".." || strings.HasPrefix(rel, "../") {
		return ""
	}
	return rel
}

// absolutePath converts a vault-relative path to an absolute path.
func (ws *workspace) absolutePath(relPath string) string {
	return filepath.Join(ws.vaultPath(), filepath.FromSlash(relPath))
}
