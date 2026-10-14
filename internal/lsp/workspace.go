package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

// workspaceCaches is an immutable snapshot of derived LSP state. Refresh builds
// a new value off the session lock and publishes it with a single Store.
type workspaceCaches struct {
	catalog     readsvc.CatalogSnapshot
	objectInfos []check.ObjectInfo
	schema      *schema.Schema
	vaultCfg    *config.VaultConfig
}

// workspace holds the resolved vault and a read-side catalog. The catalog is
// rebuilt on save and published atomically so request handlers can read it
// without waiting for reindex.
type workspace struct {
	rt *vaultruntime.Runtime

	cache atomic.Pointer[workspaceCaches]

	// Pinned copies used by request-handler snapshots. The live workspace
	// leaves these zero and serves them from cache.
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

	rt, err := vaultruntime.New(absPath, vaultruntime.Options{OpenDB: true})
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

// resolverGeneration is the cheap staleness signal ensureCachesFresh used:
// it changes when another index handle commits resolver-relevant writes.
func (ws *workspace) resolverGeneration() (int64, error) {
	if ws == nil || ws.rt == nil || ws.rt.DB == nil {
		return 0, fmt.Errorf("workspace database is not open")
	}
	return ws.rt.DB.ResolverGeneration()
}

func (ws *workspace) loaded() *workspaceCaches {
	if c := ws.cache.Load(); c != nil {
		return c
	}
	return &workspaceCaches{}
}

func (ws *workspace) vaultConfig() *config.VaultConfig {
	if c := ws.cache.Load(); c != nil && c.vaultCfg != nil {
		return c.vaultCfg
	}
	if ws.rt != nil {
		return ws.rt.VaultCfg
	}
	return nil
}

func (ws *workspace) schema() *schema.Schema {
	if c := ws.cache.Load(); c != nil && c.schema != nil {
		return c.schema
	}
	if ws.rt != nil && ws.rt.Schema != nil {
		return ws.rt.Schema
	}
	return schema.New()
}

// view returns a handler-safe snapshot that pins the current cache pointer.
func (ws *workspace) view() *workspace {
	c := ws.loaded()
	snap := &workspace{rt: ws.rt, catalog: c.catalog, objectInfos: c.objectInfos}
	snap.cache.Store(c)
	return snap
}

// refresh reloads vault config and schema from disk, incrementally reindexes
// changed files, and rebuilds derived caches. Called from the background
// save refresher, not under the session lock.
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

	sch := ws.rt.Schema
	if sch == nil {
		sch = schema.New()
	}
	ws.cache.Store(&workspaceCaches{
		catalog:     catalog,
		objectInfos: infos,
		schema:      sch,
		vaultCfg:    ws.rt.VaultCfg,
	})
	return nil
}

// newValidator builds a document validator backed by the cached index state.
func (ws *workspace) newValidator() *check.Validator {
	c := ws.loaded()
	vaultCfg := c.vaultCfg
	if vaultCfg == nil {
		vaultCfg = ws.vaultConfig()
	}
	sch := c.schema
	if sch == nil {
		sch = ws.schema()
	}
	var objectsRoot, pagesRoot, dailyDir string
	if vaultCfg != nil {
		if vaultCfg.HasDirectoriesConfig() {
			objectsRoot = vaultCfg.GetObjectsRoot()
			pagesRoot = vaultCfg.GetPagesRoot()
		}
		dailyDir = vaultCfg.GetDailyDirectory()
	}
	return check.New(check.Options{
		Schema:      sch,
		ObjectInfos: c.objectInfos,
		Aliases:     c.catalog.Aliases,
		Resolver:    c.catalog.Resolver,
		ObjectsRoot: objectsRoot,
		PagesRoot:   pagesRoot,
		DailyDir:    dailyDir,
	})
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
