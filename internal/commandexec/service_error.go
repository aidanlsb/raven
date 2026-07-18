package commandexec

import (
	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

// FromServiceError converts a service-layer error into the canonical failure
// envelope. When err carries a *svcerr.Error its code, message, suggestion, and
// details are mapped straight through; otherwise it degrades to a generic
// INTERNAL_ERROR carrying the original message. This is the single adapter
// handlers use so structured service errors map to envelopes consistently.
func FromServiceError(err error) Result {
	return FromServiceErrorWithFallback(err, "")
}

// FromServiceErrorWithFallback behaves like FromServiceError but applies
// fallbackSuggestion when the structured error carries no suggestion (and as
// the suggestion on the generic INTERNAL_ERROR degradation path).
func FromServiceErrorWithFallback(err error, fallbackSuggestion string) Result {
	if svcErr, ok := svcerr.AsError(err); ok {
		suggestion := svcErr.Suggestion
		if suggestion == "" {
			suggestion = fallbackSuggestion
		}
		return Failure(svcErr.Code, svcErr.Error(), serviceErrorDetails(svcErr.Details), suggestion)
	}
	message := ""
	if err != nil {
		message = err.Error()
	}
	return Failure(codes.ErrInternal, message, nil, fallbackSuggestion)
}

// serviceErrorDetails normalizes an empty details map to a true nil interface
// so the envelope omits the field entirely (matching handlers that previously
// passed a literal nil) rather than emitting "details": null.
func serviceErrorDetails(details map[string]any) interface{} {
	if len(details) == 0 {
		return nil
	}
	return details
}
