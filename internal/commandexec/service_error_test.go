package commandexec

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestFromServiceErrorMapsStructuredError(t *testing.T) {
	t.Parallel()

	svcErr := svcerr.New(codes.ErrObjectNotFound, "not here").
		WithSuggestion("check the id").
		WithDetails(map[string]any{"ref": "notes/a"})

	got := FromServiceError(fmt.Errorf("wrap: %w", svcErr))

	if got.OK {
		t.Fatalf("OK = true, want false")
	}
	if got.Error.Code != codes.ErrObjectNotFound {
		t.Fatalf("code = %q, want %q", got.Error.Code, codes.ErrObjectNotFound)
	}
	if got.Error.Message != "not here" {
		t.Fatalf("message = %q, want %q", got.Error.Message, "not here")
	}
	if got.Error.Suggestion != "check the id" {
		t.Fatalf("suggestion = %q, want %q", got.Error.Suggestion, "check the id")
	}
	details, ok := got.Error.Details.(map[string]any)
	if !ok || details["ref"] != "notes/a" {
		t.Fatalf("details = %#v, want map with ref notes/a", got.Error.Details)
	}
}

func TestFromServiceErrorOmitsEmptyDetails(t *testing.T) {
	t.Parallel()

	got := FromServiceError(svcerr.New(codes.ErrInvalidInput, "bad"))
	if got.Error.Details != nil {
		t.Fatalf("details = %#v, want nil so the envelope omits the field", got.Error.Details)
	}
}

func TestFromServiceErrorDegradesToInternal(t *testing.T) {
	t.Parallel()

	got := FromServiceError(errors.New("boom"))
	if got.Error.Code != codes.ErrInternal {
		t.Fatalf("code = %q, want %q", got.Error.Code, codes.ErrInternal)
	}
	if got.Error.Message != "boom" {
		t.Fatalf("message = %q, want %q", got.Error.Message, "boom")
	}
}

func TestFromServiceErrorWithFallbackSuggestion(t *testing.T) {
	t.Parallel()

	// Fallback applies when the structured error carries no suggestion...
	got := FromServiceErrorWithFallback(svcerr.New(codes.ErrConfigInvalid, "bad config"), "fix raven.yaml")
	if got.Error.Suggestion != "fix raven.yaml" {
		t.Fatalf("suggestion = %q, want fallback", got.Error.Suggestion)
	}

	// ...but never overrides an explicit suggestion.
	withSuggestion := svcerr.New(codes.ErrConfigInvalid, "bad config").WithSuggestion("explicit")
	got = FromServiceErrorWithFallback(withSuggestion, "fix raven.yaml")
	if got.Error.Suggestion != "explicit" {
		t.Fatalf("suggestion = %q, want explicit", got.Error.Suggestion)
	}
}
