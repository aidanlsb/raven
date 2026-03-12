package schemasvc

import (
	"errors"
	"os"

	"github.com/aidanlsb/raven/internal/schema"
)

type ValidateResult struct {
	Valid  bool
	Issues []string
	Types  int
	Traits int
}

func Validate(vaultPath string) (*ValidateResult, error) {
	sch, err := schema.Load(vaultPath)
	if err != nil {
		code := ErrorSchemaInvalid
		if errors.Is(err, os.ErrNotExist) {
			code = ErrorSchemaNotFound
		}
		return nil, newError(code, err.Error(), "Fix the errors and try again", nil, err)
	}

	issues := schema.ValidateSchema(sch)
	return &ValidateResult{
		Valid:  len(issues) == 0,
		Issues: issues,
		Types:  len(sch.Types),
		Traits: len(sch.Traits),
	}, nil
}
