package toolexec

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeExecutableRequired ErrorCode = "EXECUTABLE_REQUIRED"
	CodeUnknownTool        ErrorCode = "UNKNOWN_TOOL"
	CodeExecutionFailed    ErrorCode = "EXECUTION_FAILED"
	CodeInvalidJSON        ErrorCode = "INVALID_JSON"
	CodeToolReturnedError  ErrorCode = "TOOL_RETURNED_ERROR"
)

// Error is the typed error contract for tool execution failures.
type Error struct {
	Code   ErrorCode
	Tool   string
	Output string
	Err    error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	if e.Tool == "" {
		return string(e.Code)
	}
	return fmt.Sprintf("tool '%s' failed (%s)", e.Tool, e.Code)
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func IsCode(err error, code ErrorCode) bool {
	var execErr *Error
	if !errors.As(err, &execErr) {
		return false
	}
	return execErr.Code == code
}
