package readsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/schema"
)

type RuntimeOptions struct {
	OpenDB bool
	// RequireSchema controls how a schema load failure is handled.
	//
	// A missing schema.yaml is never a failure: schema.Load returns a default
	// (empty) schema with a nil error, which is always safe to continue with.
	// A load *failure* means schema.yaml exists but is unreadable or invalid.
	//
	// When RequireSchema is true, such a failure is fatal (NewRuntime returns
	// the error). Set this for operations that rely on schema for safety or
	// correctness — for example reference-resolving mutations. When false, the
	// failure is tolerated for degraded read/render behavior: Schema may be nil
	// and the error is recorded on Runtime.SchemaLoadErr so callers can surface
	// a warning instead of silently continuing.
	RequireSchema bool
}

type Runtime struct {
	VaultPath string
	VaultCfg  *config.VaultConfig
	Schema    *schema.Schema
	DB        *index.Database

	// SchemaLoadErr records a non-fatal schema load failure when the runtime
	// was built with RequireSchema=false. It is nil when the schema loaded
	// successfully (including when schema.yaml is absent). Degraded callers
	// should emit a warning rather than swallowing this.
	SchemaLoadErr error
}

func NewRuntime(vaultPath string, opts RuntimeOptions) (*Runtime, error) {
	if vaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, err
	}

	sch, schErr := schema.Load(vaultPath)
	if schErr != nil && opts.RequireSchema {
		return nil, schErr
	}

	rt := &Runtime{
		VaultPath:     vaultPath,
		VaultCfg:      vaultCfg,
		Schema:        sch,
		SchemaLoadErr: schErr,
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
