package schemasvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func runtimeSchema(rt *vaultruntime.Runtime, suggestion string) (*schema.Schema, error) {
	if err := vaultruntime.Require(rt); err != nil {
		return nil, svcerr.Wrap(codes.ErrInvalidInput, "vault path is required", err)
	}
	if rt.SchemaLoadErr != nil {
		return nil, svcerr.Wrap(codes.ErrSchemaNotFound, rt.SchemaLoadErr.Error(), rt.SchemaLoadErr).WithSuggestion(suggestion)
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaNotFound, "schema runtime is required").WithSuggestion(suggestion)
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
	loadErrorCode codes.ErrorCode,
	edit func(*schemadoc.Document) error,
) error {
	if err := editSchemaWithLoadError(rt.VaultPath, suggestion, loadErrorCode, edit); err != nil {
		return err
	}
	return reloadRuntimeSchema(rt)
}

func reloadRuntimeSchema(rt *vaultruntime.Runtime) error {
	if err := rt.ReloadSchema(true); err != nil {
		return svcerr.Wrap(codes.ErrSchemaInvalid, "failed to reload schema after update", err).WithSuggestion("Fix schema.yaml and try again")
	}
	return nil
}
