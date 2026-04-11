package svcerror

import "errors"

// Error is the shared structured error type for all *svc packages.
type Error struct {
	Code       string
	Message    string
	Suggestion string
	Cause      error
}

func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Cause != nil {
		return e.Cause.Error()
	}
	return e.Code
}

func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Cause
}

// New constructs a new Error.
func New(code, message, suggestion string, cause error) *Error {
	return &Error{Code: code, Message: message, Suggestion: suggestion, Cause: cause}
}

// As unwraps err as a *Error. Returns the error and true on success.
func As(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}
