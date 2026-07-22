// Package vaultruntime assembles the vault-scoped dependencies needed by one
// Raven operation.
package vaultruntime

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// Stage identifies the dependency-loading stage that failed while constructing
// a Runtime. Callers use it to preserve transport-specific error contracts.
type Stage string

const (
	StageConfig   Stage = "config"
	StageSchema   Stage = "schema"
	StageDatabase Stage = "database"
)

// SetupError wraps a runtime construction failure with its owning stage.
type SetupError struct {
	Stage Stage
	Err   error
}

func (e *SetupError) Error() string {
	if e == nil {
		return "vault runtime setup failed"
	}
	return fmt.Sprintf("%s setup failed: %v", e.Stage, e.Err)
}

func (e *SetupError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Options controls which dependencies a vault operation requires.
type Options struct {
	OpenDB bool
	// RequireSchema controls how a schema load failure is handled.
	//
	// A missing schema.yaml is never a failure: schema.Load returns a default
	// schema. When RequireSchema is true, an unreadable or invalid schema is
	// fatal. When false, Runtime records the failure on SchemaLoadErr so a read
	// or render path can continue with explicit degraded behavior.
	RequireSchema bool
}

// Runtime contains dependencies loaded for one vault-scoped operation. It is
// invocation-scoped: callers must Close it and must not retain it globally.
type Runtime struct {
	VaultPath    string
	VaultCfg     *config.VaultConfig
	Schema       *schema.Schema
	DB           *index.Database
	ParseOptions *parser.ParseOptions

	// SchemaLoadErr records a tolerated schema load failure.
	SchemaLoadErr error
}

// New loads a vault runtime according to opts.
func New(vaultPath string, opts Options) (*Runtime, error) {
	if vaultPath == "" {
		return nil, fmt.Errorf("vault path is required")
	}

	vaultCfg, err := config.LoadVaultConfig(vaultPath)
	if err != nil {
		return nil, &SetupError{Stage: StageConfig, Err: err}
	}

	sch, schErr := schema.Load(vaultPath)
	if schErr != nil && opts.RequireSchema {
		return nil, &SetupError{Stage: StageSchema, Err: schErr}
	}

	rt := &Runtime{
		VaultPath:     vaultPath,
		VaultCfg:      vaultCfg,
		Schema:        sch,
		ParseOptions:  parser.OptionsFromVaultConfig(vaultCfg),
		SchemaLoadErr: schErr,
	}

	if opts.OpenDB {
		if err := rt.OpenDB(); err != nil {
			return nil, err
		}
	}

	return rt, nil
}

// OpenDB opens and configures the runtime's index on first use. It is safe to
// call more than once for the same invocation.
func (r *Runtime) OpenDB() error {
	if r == nil {
		return &SetupError{Stage: StageDatabase, Err: fmt.Errorf("vault runtime is required")}
	}
	if r.DB != nil {
		return nil
	}
	db, err := index.Open(r.VaultPath)
	if err != nil {
		return &SetupError{Stage: StageDatabase, Err: err}
	}
	db.SetDailyDirectory(r.VaultCfg.GetDailyDirectory())
	r.DB = db
	return nil
}

// Close releases resources owned by the runtime.
func (r *Runtime) Close() {
	if r == nil || r.DB == nil {
		return
	}
	_ = r.DB.Close()
}
