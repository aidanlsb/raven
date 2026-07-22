package schemasvc

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

type ErrorCode = codes.ErrorCode

const (
	ErrorSchemaNotFound ErrorCode = codes.ErrSchemaNotFound
	ErrorSchemaInvalid  ErrorCode = codes.ErrSchemaInvalid
	ErrorTypeNotFound   ErrorCode = codes.ErrTypeNotFound
	ErrorTraitNotFound  ErrorCode = codes.ErrTraitNotFound
	ErrorFieldNotFound  ErrorCode = codes.ErrFieldNotFound
	ErrorObjectExists   ErrorCode = codes.ErrObjectExists
	ErrorConfigInvalid  ErrorCode = codes.ErrConfigInvalid
	ErrorInvalidInput   ErrorCode = codes.ErrInvalidInput
	ErrorValidation     ErrorCode = codes.ErrValidationFailed
	ErrorDataIntegrity  ErrorCode = codes.ErrDataIntegrityBlock
	ErrorConfirmation   ErrorCode = codes.ErrConfirmationRequired
	ErrorFileNotFound   ErrorCode = codes.ErrFileNotFound
	ErrorFileRead       ErrorCode = codes.ErrFileRead
	ErrorFileWrite      ErrorCode = codes.ErrFileWrite
	ErrorFileOutside    ErrorCode = codes.ErrFileOutsideVault
	ErrorDatabase       ErrorCode = codes.ErrDatabase
	ErrorInternal       ErrorCode = codes.ErrInternal
)

// Error is kept as a compatibility alias for schema migration callers that
// previously matched schemasvc's package-local error type.
type Error = svcerr.Error

func newError(code ErrorCode, message, suggestion string, details map[string]interface{}, cause error) *svcerr.Error {
	return &svcerr.Error{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
		Details:    details,
		Err:        cause,
	}
}
