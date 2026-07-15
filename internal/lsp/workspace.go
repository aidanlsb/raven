package lsp

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/aidanlsb/raven/internal/check"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/model"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/resolver"
	"github.com/aidanlsb/raven/internal/schema"
)

// workspace holds the resolved vault, its open index handle, and caches
// derived from the index. Caches are rebuilt on save via refresh().
type workspace struct {
	rt *readsvc.Runtime

	resolver    *resolver.Resolver
	objectInfos []check.ObjectInfo
	objects     []model.Object
	aliases     map[string]string
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

func (ws *workspace) db() *index.Database {
	return ws.rt.DB
}

// refresh reloads vault config and schema from disk, incrementally reindexes
// changed files, and rebuilds derived caches. Called after didSave.
func (ws *workspace) refresh() error {
	if vaultCfg, err := config.LoadVaultConfig(ws.rt.VaultPath); err == nil {
		ws.rt.VaultCfg = vaultCfg
	}
	if sch, err := schema.Load(ws.rt.VaultPath); err == nil {
		ws.rt.Schema = sch
	}

	if report, err := readsvc.SmartReindex(ws.rt); err != nil {
		return fmt.Errorf("reindex failed: %w", err)
	} else if len(report.Failures) > 0 {
		fmt.Fprintf(os.Stderr, "rvn lsp: reindex skipped %d file(s)\n", len(report.Failures))
	}
	return ws.rebuildCaches()
}

func (ws *workspace) rebuildCaches() error {
	db := ws.db()

	res, err := db.Resolver(index.ResolverOptions{
		DailyDirectory: ws.vaultConfig().GetDailyDirectory(),
		Schema:         ws.schema(),
	})
	if err != nil {
		return fmt.Errorf("failed to build resolver: %w", err)
	}

	objects, err := db.AllObjects()
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}
	sort.Slice(objects, func(i, j int) bool { return objects[i].ID < objects[j].ID })

	aliases, err := db.AllAliases()
	if err != nil {
		return fmt.Errorf("failed to list aliases: %w", err)
	}

	infos := make([]check.ObjectInfo, 0, len(objects))
	for _, obj := range objects {
		infos = append(infos, check.ObjectInfo{ID: obj.ID, Type: obj.Type})
	}

	ws.resolver = res
	ws.objects = objects
	ws.objectInfos = infos
	ws.aliases = aliases
	return nil
}

// newValidator builds a document validator backed by the cached index state.
func (ws *workspace) newValidator() *check.Validator {
	validator := check.NewValidatorWithTypesAliasesAndResolver(ws.schema(), ws.objectInfos, ws.aliases, ws.resolver)
	validator.SetDailyDirectoryForInference(ws.vaultConfig().GetDailyDirectory())
	if ws.vaultConfig().HasDirectoriesConfig() {
		validator.SetDirectoryRoots(ws.vaultConfig().GetObjectsRoot(), ws.vaultConfig().GetPagesRoot())
	}
	return validator
}

// parseOptions returns parser options matching the vault's directory config.
func (ws *workspace) parseOptions() *parser.ParseOptions {
	vaultCfg := ws.vaultConfig()
	if vaultCfg == nil || !vaultCfg.HasDirectoriesConfig() {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

// parseBuffer parses in-memory buffer content as the document at absPath.
func (ws *workspace) parseBuffer(content, absPath string) (*parser.ParsedDocument, error) {
	return parser.ParseDocumentWithOptions(content, absPath, ws.vaultPath(), ws.parseOptions())
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
