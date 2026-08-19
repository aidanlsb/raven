package schemasvc

import (
	"errors"
	"os"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/schema"
	"github.com/aidanlsb/raven/internal/svcerr"
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
		return nil, svcerr.New(codes.ErrInvalidInput, "vault runtime is required")
	}
	if rt.SchemaLoadErr != nil {
		code := codes.ErrSchemaInvalid
		if errors.Is(rt.SchemaLoadErr, os.ErrNotExist) {
			code = codes.ErrSchemaNotFound
		}
		return nil, svcerr.Wrap(code, rt.SchemaLoadErr.Error(), rt.SchemaLoadErr).WithSuggestion("Fix the errors and try again")
	}
	if rt.Schema == nil {
		return nil, svcerr.New(codes.ErrSchemaNotFound, "schema runtime is required").WithSuggestion("Fix the errors and try again")
	}

	issues := schema.ValidateSchema(rt.Schema)
	return &ValidateResult{
		Valid:  len(issues) == 0,
		Issues: issues,
		Types:  len(rt.Schema.Types),
		Traits: len(rt.Schema.Traits),
	}, nil
}
