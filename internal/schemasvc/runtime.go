package schemasvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemachange"
	"github.com/aidanlsb/raven/internal/schemadoc"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func init() {
	// Wire up the schemadoc invalidation hook to avoid import cycles
	schemadoc.SetRecordInvalidationHook(func(vaultPath string, beforeSchema, afterSchema *schema.Schema) (string, interface{}, error) {
		operationID, classification, err := schemachange.RecordInvalidation(vaultPath, beforeSchema, afterSchema)
		return operationID, classification, err
	})
}

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
	result, err := editSchemaWithInvalidation(rt.VaultPath, suggestion, edit)
	if err != nil {
		return err
	}
	if reloadErr := reloadRuntimeSchema(rt); reloadErr != nil {
		return reloadErr
	}
	// Apply invalidation after schema reload (when auto-reindex enabled)
	if result != nil && result.OperationID != "" {
		// Attempt to apply invalidation, but don't fail the mutation if it errors.
		// The journal entry persists and a manual reindex will recover.
		// TODO: consider returning a warning instead of silently continuing.
		_ = schemachange.ApplyInvalidation(rt, result.OperationID, result.Classification)
	}
	return nil
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
