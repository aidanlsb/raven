package schemasvc

import (
	"strings"

	"github.com/aidanlsb/raven/internal/schema"
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
