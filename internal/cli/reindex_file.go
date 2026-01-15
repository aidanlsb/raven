package cli

import (
	"os"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// reindexFile re-parses and re-indexes a single markdown file.
//
// This is used for "auto reindex" after CLI mutations. It updates:
// - objects
// - inline traits
// - refs (including resolving refs for this file)
// - date index
// - FTS
func reindexFile(vaultPath, filePath string, vaultCfg *config.VaultConfig) error {
	// Load schema
	sch, err := schema.Load(vaultPath)
	if err != nil {
		return err
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Parse with directory roots when configured
	var parseOpts *parser.ParseOptions
	if vaultCfg != nil && vaultCfg.HasDirectoriesConfig() {
		parseOpts = &parser.ParseOptions{
			ObjectsRoot: vaultCfg.GetObjectsRoot(),
			PagesRoot:   vaultCfg.GetPagesRoot(),
		}
	}
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, parseOpts)
	if err != nil {
		return err
	}

	// Best-effort mtime tracking (used for incremental reindex correctness).
	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	// Open database and index
	db, err := index.Open(vaultPath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := db.IndexDocumentWithMtime(doc, sch, mtime); err != nil {
		return err
	}

	// Resolve references for this file only (cheap enough for auto-reindex).
	dailyDir := "daily"
	if vaultCfg != nil && vaultCfg.DailyDirectory != "" {
		dailyDir = vaultCfg.DailyDirectory
	}
	_, _ = db.ResolveReferencesForFile(doc.FilePath, dailyDir)

	return nil
}
