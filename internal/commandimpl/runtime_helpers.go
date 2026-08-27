package commandimpl

import (
	"errors"
	"strings"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

const indexUpdateFailedWarningCode = codes.WarnIndexUpdateFailed

func newRequiredCommandVaultRuntime(vaultPath string, openDB bool) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{OpenDB: openDB, RequireSchema: true})
}

func newConfigCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{})
}

func newConfigOnlyCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{SkipSchema: true})
}

// newSchemaMutationCommandVaultRuntime creates a runtime for schema mutation
// commands that need to optionally run auto-reindex. It loads schema (required),
// config (best-effort), and opens the database. If config is invalid, the runtime
// creation succeeds but ApplyInvalidation will skip auto-reindex.
func newSchemaMutationCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	rt, err := vaultruntime.New(strings.TrimSpace(vaultPath), vaultruntime.Options{
		OpenDB:        true,
		RequireSchema: true,
	})
	if err == nil {
		return rt, commandexec.Result{}
	}

	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) && setupErr.Stage == vaultruntime.StageConfig {
		rt, err = vaultruntime.New(strings.TrimSpace(vaultPath), vaultruntime.Options{
			OpenDB:        true,
			SkipConfig:    true,
			RequireSchema: true,
		})
		if err == nil {
			return rt, commandexec.Result{}
		}
	}

	return nil, mapVaultRuntimeSetupFailure(err)
}

func newSchemaFirstCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		RequireSchema: true,
		SchemaFirst:   true,
	})
}

func newLazyConfigCommandRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		SkipConfig: true,
		SkipSchema: true,
	})
}

func newDatabaseCommandVaultRuntime(vaultPath string) (*vaultruntime.Runtime, commandexec.Result) {
	return newCommandVaultRuntime(vaultPath, vaultruntime.Options{
		OpenDB:     true,
		SkipConfig: true,
		SkipSchema: true,
	})
}

func newCommandVaultRuntime(vaultPath string, opts vaultruntime.Options) (*vaultruntime.Runtime, commandexec.Result) {
	rt, err := vaultruntime.New(strings.TrimSpace(vaultPath), opts)
	if err == nil {
		return rt, commandexec.Result{}
	}
	return nil, mapVaultRuntimeSetupFailure(err)
}

func mapVaultRuntimeSetupFailure(err error) commandexec.Result {
	var setupErr *vaultruntime.SetupError
	if errors.As(err, &setupErr) {
		switch setupErr.Stage {
		case vaultruntime.StageConfig:
			return commandexec.Failure("CONFIG_INVALID", "failed to load raven.yaml", nil, "Fix raven.yaml and try again")
		case vaultruntime.StageSchema:
			return commandexec.Failure("SCHEMA_INVALID", "failed to load schema", nil, "Fix schema.yaml and try again")
		case vaultruntime.StageDatabase:
			if failure, ok := mapIndexRebuildRequired(err); ok {
				return failure
			}
			return commandexec.Failure("DATABASE_ERROR", "failed to open database", nil, "Run 'rvn reindex' to rebuild the database")
		}
	}

	return commandexec.Failure("INVALID_INPUT", "vault path is required", nil, "Resolve a vault before invoking the command")
}

func appendCommandWarnings(groups ...[]commandexec.Warning) []commandexec.Warning {
	total := 0
	for _, group := range groups {
		total += len(group)
	}
	if total == 0 {
		return nil
	}
	combined := make([]commandexec.Warning, 0, total)
	for _, group := range groups {
		combined = append(combined, group...)
	}
	return combined
}
