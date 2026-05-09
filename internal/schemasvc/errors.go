package schemasvc

import "github.com/aidanlsb/raven/internal/codes"

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
	ErrorInternal       ErrorCode = codes.ErrInternal
)

type Error struct {
	Code       ErrorCode
	Message    string
	Suggestion string
	Details    map[string]interface{}
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

func newError(code ErrorCode, message, suggestion string, details map[string]interface{}, cause error) *Error {
	return &Error{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
		Details:    details,
		Cause:      cause,
	}
}
