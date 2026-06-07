package query

// ExecutionError represents a user-facing error discovered while executing a query.
// These should map to query-facing error codes rather than storage/index failures.
type ExecutionError struct {
	Message    string
	Suggestion string
	Err        error
}

func (e *ExecutionError) Error() string {
	if e == nil {
		return ""
	}
	if e.Message != "" {
		return e.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return "query execution failed"
}

func (e *ExecutionError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newExecutionError(message, suggestion string, err error) *ExecutionError {
	return &ExecutionError{Message: message, Suggestion: suggestion, Err: err}
}
