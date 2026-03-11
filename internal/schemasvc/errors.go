package schemasvc

type ErrorCode string

const (
	ErrorSchemaNotFound ErrorCode = "SCHEMA_NOT_FOUND"
	ErrorSchemaInvalid  ErrorCode = "SCHEMA_INVALID"
	ErrorTypeNotFound   ErrorCode = "TYPE_NOT_FOUND"
	ErrorConfigInvalid  ErrorCode = "CONFIG_INVALID"
	ErrorInvalidInput   ErrorCode = "INVALID_INPUT"
	ErrorFileNotFound   ErrorCode = "FILE_NOT_FOUND"
	ErrorFileRead       ErrorCode = "FILE_READ_ERROR"
	ErrorFileWrite      ErrorCode = "FILE_WRITE_ERROR"
	ErrorFileOutside    ErrorCode = "FILE_OUTSIDE_VAULT"
	ErrorInternal       ErrorCode = "INTERNAL_ERROR"
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
