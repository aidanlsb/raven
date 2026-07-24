package checkfixsvc

import (
	"fmt"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func validationError(err error) *svcerr.Error {
	if err == nil {
		return nil
	}
	return svcerr.Wrap(codes.ErrValidationFailed, err.Error(), err)
}

func validationErrorf(format string, args ...any) *svcerr.Error {
	return validationError(fmt.Errorf(format, args...))
}
