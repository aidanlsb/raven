package bulkops

import "errors"

type ErrorCode string

const (
	CodeInvalidInput    ErrorCode = "INVALID_INPUT"
	CodeMissingArgument ErrorCode = "MISSING_ARGUMENT"
)

type Error struct {
	Code       ErrorCode
	Message    string
	Suggestion string
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

func newError(code ErrorCode, message, suggestion string) error {
	return &Error{
		Code:       code,
		Message:    message,
		Suggestion: suggestion,
	}
}

func AsError(err error) (*Error, bool) {
	var target *Error
	if !errors.As(err, &target) {
		return nil, false
	}
	return target, true
}
