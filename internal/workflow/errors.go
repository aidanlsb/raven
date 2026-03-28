package workflow

import "errors"

// Code is a stable workflow-domain error code used by CLI/MCP adapters.
type Code string

const (
	CodeInvalidInput                Code = "INVALID_INPUT"
	CodeDuplicateName               Code = "DUPLICATE_NAME"
	CodeRefNotFound                 Code = "REF_NOT_FOUND"
	CodeFileNotFound                Code = "FILE_NOT_FOUND"
	CodeFileExists                  Code = "FILE_EXISTS"
	CodeFileReadError               Code = "FILE_READ_ERROR"
	CodeFileWriteError              Code = "FILE_WRITE_ERROR"
	CodeFileOutsideVault            Code = "FILE_OUTSIDE_VAULT"
	CodeWorkflowNotFound            Code = "WORKFLOW_NOT_FOUND"
	CodeWorkflowInvalid             Code = "WORKFLOW_INVALID"
	CodeWorkflowChanged             Code = "WORKFLOW_CHANGED"
	CodeWorkflowRunNotFound         Code = "WORKFLOW_RUN_NOT_FOUND"
	CodeWorkflowNotAwaitingAgent    Code = "WORKFLOW_NOT_AWAITING_AGENT"
	CodeWorkflowTerminalState       Code = "WORKFLOW_TERMINAL_STATE"
	CodeWorkflowConflict            Code = "WORKFLOW_CONFLICT"
	CodeWorkflowStateCorrupt        Code = "WORKFLOW_STATE_CORRUPT"
	CodeWorkflowInputInvalid        Code = "WORKFLOW_INPUT_INVALID"
	CodeWorkflowAgentOutputInvalid  Code = "WORKFLOW_AGENT_OUTPUT_INVALID"
	CodeWorkflowInterpolationError  Code = "WORKFLOW_INTERPOLATION_ERROR"
	CodeWorkflowToolExecutionFailed Code = "WORKFLOW_TOOL_EXECUTION_FAILED"
)

// DomainError carries workflow-domain error code and structured context.
type DomainError struct {
	Code    Code
	Message string
	StepID  string
	Details map[string]interface{}
	Err     error
}

func (e *DomainError) Error() string {
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

func (e *DomainError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func newDomainError(code Code, msg string, err error) *DomainError {
	return &DomainError{
		Code:    code,
		Message: msg,
		Err:     err,
	}
}

func newStepDomainError(code Code, stepID, msg string, err error) *DomainError {
	de := newDomainError(code, msg, err)
	de.StepID = stepID
	return de
}

func withStepDomainError(err error, code Code, stepID string) error {
	if err == nil {
		return nil
	}
	if de, ok := AsDomainError(err); ok {
		if de.StepID != "" || stepID == "" {
			return de
		}
		clone := *de
		clone.StepID = stepID
		return &clone
	}
	return newStepDomainError(code, stepID, err.Error(), err)
}

func AsDomainError(err error) (*DomainError, bool) {
	if err == nil {
		return nil, false
	}
	var de *DomainError
	if errors.As(err, &de) {
		return de, true
	}
	return nil, false
}
