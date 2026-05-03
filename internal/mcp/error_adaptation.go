package mcp

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func adaptCanonicalResultForMCP(commandID string, rawArgs map[string]interface{}, result commandexec.Result) commandexec.Result {
	if result.OK || result.Error == nil {
		return result
	}

	errCopy := *result.Error
	errCopy.Suggestion = adaptErrorSuggestionForMCP(commandID, errCopy)
	errCopy.Details = adaptValidationDetailsForMCP(commandID, rawArgs, errCopy)
	result.Error = &errCopy
	return result
}

func adaptValidationDetailsForMCP(commandID string, rawArgs map[string]interface{}, errInfo commandexec.ErrorInfo) interface{} {
	if errInfo.Code != "INVALID_ARGS" {
		return errInfo.Details
	}

	issues, ok := validationIssuesFromDetails(errInfo.Details)
	if !ok {
		return errInfo.Details
	}

	return detailsWithValidationMetadata(commandID, errInfo.Details, withCommandArgumentHints(commandID, rawArgs, issues))
}

func adaptErrorSuggestionForMCP(commandID string, err commandexec.ErrorInfo) string {
	describeSuggestion := describeRetrySuggestion(commandID)
	suggestion := strings.TrimSpace(err.Suggestion)
	cliSpecific := suggestion == "Check command arguments and retry" || strings.HasPrefix(suggestion, "Usage: rvn ")

	switch err.Code {
	case "INVALID_ARGS", "MISSING_ARGUMENT":
		if suggestion != "" && !cliSpecific {
			return suggestion
		}
		if describeSuggestion != "" {
			return describeSuggestion
		}
	case "QUERY_INVALID":
		if suggestion == "" {
			return "Check the query syntax, quote string literals, and retry."
		}
	}

	if suggestion == "" {
		return ""
	}
	if describeSuggestion != "" && cliSpecific {
		return describeSuggestion
	}
	return suggestion
}

func describeRetrySuggestion(commandID string) string {
	commandID = strings.TrimSpace(commandID)
	if commandID == "" {
		return ""
	}
	return fmt.Sprintf("Call %s with command '%s' for the strict contract and retry", compactToolDescribe, commandID)
}

func validationIssuesFromDetails(details interface{}) ([]validationIssue, bool) {
	detailMap, ok := details.(map[string]interface{})
	if !ok {
		return nil, false
	}

	switch value := detailMap["issues"].(type) {
	case []validationIssue:
		out := make([]validationIssue, len(value))
		copy(out, value)
		return out, true
	case []interface{}:
		out := make([]validationIssue, 0, len(value))
		for _, item := range value {
			issue, ok := item.(validationIssue)
			if !ok {
				return nil, false
			}
			out = append(out, issue)
		}
		return out, true
	default:
		return nil, false
	}
}

func detailsWithValidationMetadata(commandID string, details interface{}, issues []validationIssue) interface{} {
	detailMap, ok := details.(map[string]interface{})
	if !ok {
		return details
	}

	out := make(map[string]interface{}, len(detailMap))
	for key, value := range detailMap {
		out[key] = value
	}
	out["issues"] = issues
	if contract, ok := buildCommandContract(commandID); ok {
		out["args_schema"] = compactArgsSchema(contract)
		out["schema_hash"] = contract.SchemaHash
		out["invoke_shape"] = compactInvokeShape()
	}
	return out
}
