package mcp

import (
	"fmt"
	"strings"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func adaptCanonicalResultForMCP(commandID string, result commandexec.Result) commandexec.Result {
	if result.OK || result.Error == nil {
		return result
	}

	errCopy := *result.Error
	errCopy.Suggestion = adaptErrorSuggestionForMCP(commandID, errCopy)
	result.Error = &errCopy
	return result
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
