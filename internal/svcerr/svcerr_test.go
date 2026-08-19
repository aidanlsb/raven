package svcerr

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
)

func TestErrorMessageFallback(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		err  *Error
		want string
	}{
		{name: "message wins", err: &Error{Code: codes.ErrInvalidInput, Message: "bad", Err: errors.New("cause")}, want: "bad"},
		{name: "cause fallback", err: &Error{Code: codes.ErrInvalidInput, Err: errors.New("cause")}, want: "cause"},
		{name: "code fallback", err: &Error{Code: codes.ErrInvalidInput}, want: "INVALID_INPUT"},
		{name: "nil receiver", err: nil, want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.err.Error(); got != tc.want {
				t.Fatalf("Error() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAsErrorUnwrapsChain(t *testing.T) {
	t.Parallel()

	base := New(codes.ErrObjectNotFound, "missing")
	wrapped := fmt.Errorf("outer: %w", base)

	got, ok := AsError(wrapped)
	if !ok {
		t.Fatalf("AsError() ok = false, want true")
	}
	if got.Code != codes.ErrObjectNotFound {
		t.Fatalf("AsError() code = %q, want %q", got.Code, codes.ErrObjectNotFound)
	}

	if _, ok := AsError(errors.New("plain")); ok {
		t.Fatalf("AsError() ok = true for plain error, want false")
	}
}

func TestConstructorsAndBuilders(t *testing.T) {
	t.Parallel()

	cause := errors.New("cause")
	err := Wrap(codes.ErrFileWrite, "write failed", cause).
		WithSuggestion("try again").
		WithDetails(map[string]any{"path": "notes/a.md"})

	if err.Code != codes.ErrFileWrite {
		t.Fatalf("code = %q, want %q", err.Code, codes.ErrFileWrite)
	}
	if err.Suggestion != "try again" {
		t.Fatalf("suggestion = %q, want %q", err.Suggestion, "try again")
	}
	if err.Details["path"] != "notes/a.md" {
		t.Fatalf("details path = %v, want notes/a.md", err.Details["path"])
	}
	if !errors.Is(err, cause) {
		t.Fatalf("errors.Is(err, cause) = false, want true")
	}
}

func TestValidationError(t *testing.T) {
	t.Parallel()

	cause := errors.New("walk failed")
	err := ValidationError(cause)
	if err.Code != codes.ErrValidationFailed || err.Message != cause.Error() {
		t.Fatalf("ValidationError() = %#v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatal("ValidationError() should preserve its cause")
	}
	if ValidationError(nil) != nil {
		t.Fatal("ValidationError(nil) should return nil")
	}
}
