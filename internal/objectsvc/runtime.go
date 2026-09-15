package objectsvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func requireRuntime(rt *vaultruntime.Runtime) error {
	if err := vaultruntime.Require(rt); err != nil {
		return svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	return nil
}

func requireVaultConfig(rt *vaultruntime.Runtime) error {
	if err := requireRuntime(rt); err != nil {
		return err
	}
	if rt.VaultCfg == nil {
		return svcerr.New(codes.ErrValidationFailed, "vault config is required").WithSuggestion("Fix raven.yaml and try again")
	}
	return nil
}

func requireSchema(rt *vaultruntime.Runtime) error {
	if rt.Schema == nil {
		return svcerr.New(codes.ErrValidationFailed, "schema is required").WithSuggestion("Fix schema.yaml and try again")
	}
	return nil
}

func objectsRoot(rt *vaultruntime.Runtime) string {
	if rt == nil || rt.VaultCfg == nil {
		return ""
	}
	return rt.VaultCfg.GetObjectsRoot()
}

func pagesRoot(rt *vaultruntime.Runtime) string {
	if rt == nil || rt.VaultCfg == nil {
		return ""
	}
	return rt.VaultCfg.GetPagesRoot()
}

func templateDir(rt *vaultruntime.Runtime) string {
	if rt == nil || rt.VaultCfg == nil {
		return ""
	}
	return rt.VaultCfg.GetTemplateDirectory()
}
