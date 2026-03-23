package commandimpl

import (
	"os"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

func buildParseOptions(vaultCfg *config.VaultConfig) *parser.ParseOptions {
	if vaultCfg == nil {
		return nil
	}
	return &parser.ParseOptions{
		ObjectsRoot: vaultCfg.GetObjectsRoot(),
		PagesRoot:   vaultCfg.GetPagesRoot(),
	}
}

func autoReindexFile(vaultPath, filePath string, vaultCfg *config.VaultConfig) {
	if vaultCfg == nil || !vaultCfg.IsAutoReindexEnabled() {
		return
	}

	sch, err := schema.Load(vaultPath)
	if err != nil {
		return
	}

	content, err := os.ReadFile(filePath)
	if err != nil {
		return
	}

	doc, err := parser.ParseDocumentWithOptions(string(content), filePath, vaultPath, buildParseOptions(vaultCfg))
	if err != nil {
		return
	}

	var mtime int64
	if st, err := os.Stat(filePath); err == nil {
		mtime = st.ModTime().Unix()
	}

	db, err := index.Open(vaultPath)
	if err != nil {
		return
	}
	defer db.Close()
	db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	_ = db.IndexDocumentWithMtime(doc, sch, mtime)
}
