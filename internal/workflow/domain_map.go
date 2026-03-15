package workflow

// DomainCodeToErrorCode maps workflow domain error codes to stable CLI/MCP error codes.
func DomainCodeToErrorCode(code Code) string {
	switch code {
	case CodeInvalidInput:
		return "INVALID_INPUT"
	case CodeDuplicateName:
		return "DUPLICATE_NAME"
	case CodeRefNotFound:
		return "REF_NOT_FOUND"
	case CodeFileNotFound:
		return "FILE_NOT_FOUND"
	case CodeFileExists:
		return "FILE_EXISTS"
	case CodeFileReadError:
		return "FILE_READ_ERROR"
	case CodeFileWriteError:
		return "FILE_WRITE_ERROR"
	case CodeFileOutsideVault:
		return "FILE_OUTSIDE_VAULT"
	case CodeWorkflowNotFound:
		return "WORKFLOW_NOT_FOUND"
	case CodeWorkflowInvalid:
		return "WORKFLOW_INVALID"
	case CodeWorkflowChanged:
		return "WORKFLOW_CHANGED"
	case CodeWorkflowRunNotFound:
		return "WORKFLOW_RUN_NOT_FOUND"
	case CodeWorkflowNotAwaitingAgent:
		return "WORKFLOW_NOT_AWAITING_AGENT"
	case CodeWorkflowTerminalState:
		return "WORKFLOW_TERMINAL_STATE"
	case CodeWorkflowConflict:
		return "WORKFLOW_CONFLICT"
	case CodeWorkflowStateCorrupt:
		return "WORKFLOW_STATE_CORRUPT"
	case CodeWorkflowInputInvalid:
		return "WORKFLOW_INPUT_INVALID"
	case CodeWorkflowAgentOutputInvalid:
		return "WORKFLOW_AGENT_OUTPUT_INVALID"
	case CodeWorkflowInterpolationError:
		return "WORKFLOW_INTERPOLATION_ERROR"
	case CodeWorkflowToolExecutionFailed:
		return "WORKFLOW_TOOL_EXECUTION_FAILED"
	default:
		return "INTERNAL_ERROR"
	}
}

// DomainCodeHint returns the default user-facing hint for a workflow domain code.
func DomainCodeHint(code Code) string {
	switch code {
	case CodeWorkflowNotFound:
		return "Use 'rvn workflow list' to see available workflows"
	case CodeRefNotFound:
		return "Use 'rvn workflow show <name>' to inspect step ids"
	case CodeWorkflowChanged:
		return "Start a new run to use the latest workflow definition"
	case CodeWorkflowConflict:
		return "Fetch latest run state and retry"
	case CodeWorkflowAgentOutputInvalid:
		return "Provide valid agent output JSON with top-level 'outputs'"
	default:
		return ""
	}
}
