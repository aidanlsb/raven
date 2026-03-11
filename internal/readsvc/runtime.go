package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
)

type RuntimeOptions struct {
	OpenDB bool
}

type Runtime struct {
	VaultPath string
	VaultCfg  *config.VaultConfig
	Schema    *schema.Schema
	DB        *index.Database
}

func NewRuntime(vaultPath string, opts RuntimeOptions) (*Runtime, error) {
	if vaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, err
	}

	// Schema load failures are not fatal for read/query operations.
	sch, _ := schema.Load(vaultPath)

	rt := &Runtime{
		VaultPath: vaultPath,
		VaultCfg:  vaultCfg,
		Schema:    sch,
	}

	if opts.OpenDB {
		db, err := index.Open(vaultPath)
		if err != nil {
			return nil, err
		}
		db.SetDailyDirectory(vaultCfg.GetDailyDirectory())
		rt.DB = db
	}

	return rt, nil
}

func (r *Runtime) Close() {
	if r == nil || r.DB == nil {
		return
	}
	_ = r.DB.Close()
}
