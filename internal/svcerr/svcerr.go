// Package svcerr defines Raven's shared, transport-neutral structured service
// error. Operation packages return it instead of maintaining their own
// near-identical Error/Code/AsError shapes, and handlers convert it to a response
// envelope through a single adapter (commandexec.FromServiceError).
package svcerr

import (
	"errors"

	"github.com/aidanlsb/raven/internal/codes"
)

// Error is the canonical structured error returned by operation packages. It
// carries a stable ErrorCode plus the optional message, suggestion, structured
// details, and wrapped cause that handlers surface in the JSON envelope.
type Error struct {
	Code       codes.ErrorCode
	Message    string
	Suggestion string
	Details    map[string]any
	Err        error
}

// Error implements the error interface. It prefers the human-readable message,
// falling back to the wrapped cause and finally the stable code so a *Error is
// never rendered as an empty string.
func (e *Error) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return string(e.Code)
}

// Unwrap exposes the wrapped cause for errors.Is / errors.As traversal.
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// New builds a structured error with a code and message.
func New(code codes.ErrorCode, message string) *Error {
	return &Error{Code: code, Message: message}
}

// Wrap builds a structured error that also carries an underlying cause.
func Wrap(code codes.ErrorCode, message string, err error) *Error {
	return &Error{Code: code, Message: message, Err: err}
}

// ValidationError converts a validation failure into Raven's shared structured
// error contract. A nil cause remains nil.
func ValidationError(err error) *Error {
	if err == nil {
		return nil
	}
	return Wrap(codes.ErrValidationFailed, err.Error(), err)
}

// WithSuggestion sets the remediation suggestion and returns the receiver so
// constructors can be chained.
func (e *Error) WithSuggestion(suggestion string) *Error {
	if e == nil {
		return nil
	}
	e.Suggestion = suggestion
	return e
}

// WithDetails sets structured details and returns the receiver so constructors
// can be chained.
func (e *Error) WithDetails(details map[string]any) *Error {
	if e == nil {
		return nil
	}
	e.Details = details
	return e
}

// AsError extracts a *Error from err's chain, reporting whether one was found.
func AsError(err error) (*Error, bool) {
	var svcErr *Error
	if errors.As(err, &svcErr) {
		return svcErr, true
	}
	return nil, false
}
