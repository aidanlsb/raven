// Package vaultruntime assembles the vault-scoped dependencies needed by one
// Raven operation.
package vaultruntime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/index"
	"github.com/aidanlsb/raven/internal/parseopts"
	"github.com/aidanlsb/raven/internal/parser"
	"github.com/aidanlsb/raven/internal/schema"
)

// ErrVaultPathRequired reports a missing vault path at a service boundary.
var ErrVaultPathRequired = errors.New("vault path is required")

// Require validates that rt identifies a vault.
func Require(rt *Runtime) error {
	if rt == nil {
		return ErrVaultPathRequired
	}
	return RequirePath(rt.VaultPath)
}

// RequirePath validates a vault path for compatibility service boundaries that
// do not yet receive a Runtime.
func RequirePath(vaultPath string) error {
	if strings.TrimSpace(vaultPath) == "" {
		return ErrVaultPathRequired
	}
	return nil
}

// Stage identifies the dependency-loading stage that failed while constructing
// a Runtime. Callers use it to preserve transport-specific error contracts.
type Stage string

const (
	StageConfig   Stage = "config"
	StageSchema   Stage = "schema"
	StageDatabase Stage = "database"
)

// SetupFailure further classifies failures whose transport mapping differs
// within a dependency-loading stage.
type SetupFailure string

const (
	SetupFailureIndexRebuildRequired SetupFailure = "index_rebuild_required"
)

// SetupError wraps a runtime construction failure with its owning stage.
type SetupError struct {
	Stage   Stage
	Failure SetupFailure
	Err     error
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
	// SkipConfig avoids loading raven.yaml for operations that only need the
	// database and do not interpret vault paths.
	SkipConfig bool
	// SkipSchema avoids loading schema.yaml for operations that are explicitly
	// schema-free, such as vault-config editing and database statistics.
	SkipSchema bool
	// SchemaFirst preserves operations whose historical setup contract loaded
	// schema before config. It only affects which setup failure wins when both
	// files are invalid.
	SchemaFirst bool
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
	VaultPath         string
	VaultConfigPath   string
	VaultConfigExists bool
	VaultCfg          *config.VaultConfig
	Schema            *schema.Schema
	DB                *index.Database
	ParseOptions      *parser.ParseOptions

	// SchemaLoadErr records a tolerated schema load failure.
	SchemaLoadErr error
}

// FromRequest reuses rt when provided or constructs a runtime from the request
// dependencies. The returned bool reports whether a new runtime was constructed.
func FromRequest(
	rt *Runtime,
	vaultPath string,
	vaultCfg *config.VaultConfig,
	sch *schema.Schema,
	parseOptions *parser.ParseOptions,
) (*Runtime, bool) {
	if rt != nil {
		return rt, false
	}
	return &Runtime{
		VaultPath:    vaultPath,
		VaultCfg:     vaultCfg,
		Schema:       sch,
		ParseOptions: parseOptions,
	}, true
}

// New loads a vault runtime according to opts.
func New(vaultPath string, opts Options) (*Runtime, error) {
	if err := RequirePath(vaultPath); err != nil {
		return nil, err
	}

	rt := &Runtime{
		VaultPath:       vaultPath,
		VaultConfigPath: filepath.Join(vaultPath, "raven.yaml"),
	}
	loadConfig := func() error {
		if opts.SkipConfig {
			return nil
		}
		return rt.ReloadConfig()
	}
	loadSchema := func() error {
		if opts.SkipSchema {
			return nil
		}
		return rt.ReloadSchema(opts.RequireSchema)
	}
	if opts.SchemaFirst {
		if err := loadSchema(); err != nil {
			return nil, err
		}
		if err := loadConfig(); err != nil {
			return nil, err
		}
	} else {
		if err := loadConfig(); err != nil {
			return nil, err
		}
		if err := loadSchema(); err != nil {
			return nil, err
		}
	}

	if opts.OpenDB {
		if err := rt.OpenDB(); err != nil {
			return nil, err
		}
	}

	return rt, nil
}

// ReloadConfig refreshes the runtime's vault config and derived parse options.
// Long-lived callers use it after raven.yaml changes; command runtimes normally
// load config only once through New.
func (r *Runtime) ReloadConfig() error {
	if r == nil || r.VaultPath == "" {
		return &SetupError{Stage: StageConfig, Err: fmt.Errorf("vault runtime is required")}
	}
	if r.VaultConfigPath == "" {
		r.VaultConfigPath = filepath.Join(r.VaultPath, "raven.yaml")
	}
	_, statErr := os.Stat(r.VaultConfigPath)
	r.VaultConfigExists = statErr == nil
	if statErr != nil && !os.IsNotExist(statErr) {
		return &SetupError{Stage: StageConfig, Err: statErr}
	}

	vaultCfg, err := config.LoadVaultConfig(r.VaultPath)
	if err != nil {
		return &SetupError{Stage: StageConfig, Err: err}
	}
	r.VaultCfg = vaultCfg
	r.ParseOptions = parseopts.FromVaultConfig(vaultCfg)
	if r.DB != nil {
		r.DB.SetDailyDirectory(vaultCfg.GetDailyDirectory())
	}
	return nil
}

// ReloadSchema refreshes the runtime's schema. A tolerated failure is recorded
// on SchemaLoadErr and clears Schema; required failures are returned.
func (r *Runtime) ReloadSchema(require bool) error {
	if r == nil || r.VaultPath == "" {
		return &SetupError{Stage: StageSchema, Err: fmt.Errorf("vault runtime is required")}
	}
	sch, err := schema.Load(r.VaultPath)
	r.SchemaLoadErr = err
	if err == nil {
		r.Schema = sch
	}
	if err != nil && require {
		return &SetupError{Stage: StageSchema, Err: err}
	}
	return nil
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
		setupErr := &SetupError{Stage: StageDatabase, Err: err}
		if errors.Is(err, index.ErrIndexRebuildRequired) {
			setupErr.Failure = SetupFailureIndexRebuildRequired
		}
		return setupErr
	}
	if r.VaultCfg != nil {
		db.SetDailyDirectory(r.VaultCfg.GetDailyDirectory())
	}
	r.DB = db
	return nil
}

// Close releases resources owned by the runtime.
func (r *Runtime) Close() {
	if r == nil || r.DB == nil {
		return
	}
	_ = r.DB.Close()
	r.DB = nil
}
