package bulkops

import (
	"errors"

	"github.com/aidanlsb/raven/internal/codes"
)

type ErrorCode = codes.ErrorCode

const (
	CodeInvalidInput    ErrorCode = codes.ErrInvalidInput
	CodeMissingArgument ErrorCode = codes.ErrMissingArgument
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
