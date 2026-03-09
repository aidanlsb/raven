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
func reindexFile(keepPath, filePath string, keepCfg *config.KeepConfig) error {
	// Load schema
	sch, err := schema.Load(keepPath)
	if err != nil {
		return err
	}

	// Read file
	content, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	// Parse with directory roots when configured
	parseOpts := buildParseOptions(keepCfg)
	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, keepPath, parseOpts)
	if err != nil {
		return err
	}

	// Best-effort mtime tracking (used for incremental reindex correctness).
	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	// Open database and index
	db, err := index.Open(keepPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetDailyDirectory(keepCfg.GetDailyDirectory())

	if err := db.IndexDocumentWithMtime(doc, sch, mtime); err != nil {
		return err
	}

	return nil
}
