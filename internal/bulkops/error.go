package bulkops

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func applyErr(code codes.ErrorCode, message, suggestion string) error {
	return svcerr.New(code, message).WithSuggestion(suggestion)
}
