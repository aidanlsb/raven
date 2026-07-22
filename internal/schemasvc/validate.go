package schemasvc

import (
	"errors"
	"os"

	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

type ValidateResult struct {
	Valid  bool
	Issues []string
	Types  int
	Traits int
}

func Validate(rt *vaultruntime.Runtime) (*ValidateResult, error) {
	if rt == nil {
		return nil, newError(ErrorInvalidInput, "vault runtime is required", "", nil, nil)
	}
	if rt.SchemaLoadErr != nil {
		code := ErrorSchemaInvalid
		if errors.Is(rt.SchemaLoadErr, os.ErrNotExist) {
			code = ErrorSchemaNotFound
		}
		return nil, newError(code, rt.SchemaLoadErr.Error(), "Fix the errors and try again", nil, rt.SchemaLoadErr)
	}
	if rt.Schema == nil {
		return nil, newError(ErrorSchemaNotFound, "schema runtime is required", "Fix the errors and try again", nil, nil)
	}

	issues := schema.ValidateSchema(rt.Schema)
	return &ValidateResult{
		Valid:  len(issues) == 0,
		Issues: issues,
		Types:  len(rt.Schema.Types),
		Traits: len(rt.Schema.Traits),
	}, nil
}
