package schemasvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/readsvc"
	"github.com/aidanlsb/raven/internal/reindexsvc"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/schemachange"
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

func editRuntimeSchema(rt *vaultruntime.Runtime, suggestion string, edit func(*schemadoc.Document) error) ([]Warning, error) {
	return editRuntimeSchemaRefreshing(rt, suggestion, codes.ErrSchemaNotFound, edit, readsvc.ReindexForSchemaChange)
}

func editRuntimeSchemaWithLoadError(
	rt *vaultruntime.Runtime,
	suggestion string,
	loadErrorCode codes.ErrorCode,
	edit func(*schemadoc.Document) error,
) ([]Warning, error) {
	return editRuntimeSchemaRefreshing(rt, suggestion, loadErrorCode, edit, readsvc.ReindexForSchemaChange)
}

func editRuntimeSchemaRefreshing(
	rt *vaultruntime.Runtime,
	suggestion string,
	loadErrorCode codes.ErrorCode,
	edit func(*schemadoc.Document) error,
	reindex schemachange.ReindexFunc,
) ([]Warning, error) {
	result, err := editSchemaWithInvalidationAndLoadError(rt.VaultPath, suggestion, loadErrorCode, edit)
	if err != nil {
		return nil, err
	}
	if reloadErr := reloadRuntimeSchema(rt); reloadErr != nil {
		return nil, reloadErr
	}
	return applySchemaInvalidation(rt, result, reindex), nil
}

func applySchemaInvalidation(rt *vaultruntime.Runtime, result *schemadoc.EditResult, reindex schemachange.ReindexFunc) []Warning {
	if result == nil || result.OperationID == "" {
		return nil
	}
	if err := schemachange.ApplyInvalidation(rt, result.OperationID, result.Classification, reindex); err != nil {
		return []Warning{NewIndexUpdateFailedWarning(err)}
	}
	return nil
}

// NewIndexUpdateFailedWarning reports that the schema write succeeded but the
// automatic index refresh did not. The journal entry remains for `rvn reindex`.
func NewIndexUpdateFailedWarning(err error) Warning {
	return Warning{
		Code:    codes.WarnIndexUpdateFailed,
		Message: fmt.Sprintf("auto-reindex failed after schema change: %v", err),
		Ref:     reindexsvc.IndexUpdateFailedWarningRef,
	}
}

func reloadRuntimeSchema(rt *vaultruntime.Runtime) error {
	if err := rt.ReloadSchema(true); err != nil {
		return svcerr.Wrap(codes.ErrSchemaInvalid, "failed to reload schema after update", err).WithSuggestion("Fix schema.yaml and try again")
	}
	return nil
}
