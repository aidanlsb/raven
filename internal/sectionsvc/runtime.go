package sectionsvc

import (
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func requestRuntime(
	rt *vaultruntime.Runtime,
	vaultPath string,
	vaultCfg *config.VaultConfig,
	sch *schema.Schema,
	parseOptions *parser.ParseOptions,
) (*vaultruntime.Runtime, bool) {
	if rt != nil {
		return rt, false
	}
	return &vaultruntime.Runtime{
		VaultPath:    vaultPath,
		VaultCfg:     vaultCfg,
		Schema:       sch,
		ParseOptions: parseOptions,
	}, true
}
