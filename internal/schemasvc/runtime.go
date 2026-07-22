package schemasvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func runtimeSchema(rt *vaultruntime.Runtime, suggestion string) (*schema.Schema, error) {
	if rt == nil || strings.TrimSpace(rt.VaultPath) == "" {
		return nil, newError(ErrorInvalidInput, "vault path is required", "", nil, nil)
	}
	if rt.SchemaLoadErr != nil {
		return nil, newError(ErrorSchemaNotFound, rt.SchemaLoadErr.Error(), suggestion, nil, rt.SchemaLoadErr)
	}
	if rt.Schema == nil {
		return nil, newError(ErrorSchemaNotFound, "schema runtime is required", suggestion, nil, nil)
	}
	return rt.Schema, nil
}

func editRuntimeSchema(rt *vaultruntime.Runtime, suggestion string, edit func(*schemadoc.Document) error) error {
	if err := editSchema(rt.VaultPath, suggestion, edit); err != nil {
		return err
	}
	return reloadRuntimeSchema(rt)
}

func editRuntimeSchemaWithLoadError(
	rt *vaultruntime.Runtime,
	suggestion string,
	loadErrorCode ErrorCode,
	edit func(*schemadoc.Document) error,
) error {
	if err := editSchemaWithLoadError(rt.VaultPath, suggestion, loadErrorCode, edit); err != nil {
		return err
	}
	return reloadRuntimeSchema(rt)
}

func reloadRuntimeSchema(rt *vaultruntime.Runtime) error {
	if err := rt.ReloadSchema(true); err != nil {
		return newError(ErrorSchemaInvalid, "failed to reload schema after update", "Fix schema.yaml and try again", nil, err)
	}
	return nil
}
