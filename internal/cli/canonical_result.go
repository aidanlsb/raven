package cli

import "github.com/aidanlsb/raven/internal/commandexec"

func outputCanonicalResultJSON(result commandexec.Result) {
	outputJSON(result)
}

func handleCanonicalFailure(result commandexec.Result) error {
	if result.Error == nil {
		return nil
	}
	if result.Error.Details != nil {
		return handleErrorWithDetails(result.Error.Code, result.Error.Message, result.Error.Suggestion, result.Error.Details)
	}
	return handleErrorMsg(result.Error.Code, result.Error.Message, result.Error.Suggestion)
}
