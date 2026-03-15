package editsvc

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func assertServiceCode(t *testing.T, err error, want Code) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error code %q, got nil", want)
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected editsvc error, got %T: %v", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %q, want %q", svcErr.Code, want)
	}
	return svcErr
}

func TestParseEditsJSON(t *testing.T) {
	t.Run("parses valid payload", func(t *testing.T) {
		edits, err := ParseEditsJSON(`{"edits":[{"old_str":"from","new_str":"to"}]}`)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []EditSpec{{OldStr: "from", NewStr: "to"}}
		if !reflect.DeepEqual(edits, want) {
			t.Fatalf("edits = %#v, want %#v", edits, want)
		}
	})

	t.Run("invalid payload returns INVALID_INPUT", func(t *testing.T) {
		_, err := ParseEditsJSON(`{"edits":[{"new_str":"to"}]}`)
		svcErr := assertServiceCode(t, err, CodeInvalidInput)
		if !strings.Contains(svcErr.Suggestion, "--edits-json") {
			t.Fatalf("expected suggestion to include --edits-json guidance, got %q", svcErr.Suggestion)
		}
	})

	t.Run("trailing JSON returns INVALID_INPUT", func(t *testing.T) {
		_, err := ParseEditsJSON(`{"edits":[{"old_str":"a","new_str":"b"}]} {"extra":true}`)
		assertServiceCode(t, err, CodeInvalidInput)
	})
}

func TestApplyEditsInMemory(t *testing.T) {
	t.Run("applies ordered edits", func(t *testing.T) {
		content := "alpha\nbeta\ngamma\n"
		updated, results, err := ApplyEditsInMemory(content, "notes/test.md", []EditSpec{
			{OldStr: "beta", NewStr: "delta"},
			{OldStr: "gamma", NewStr: "epsilon"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(updated, "delta") || !strings.Contains(updated, "epsilon") {
			t.Fatalf("updated content missing expected replacements: %q", updated)
		}
		if len(results) != 2 {
			t.Fatalf("expected 2 edit results, got %d", len(results))
		}
		if results[0].Line != 2 {
			t.Fatalf("expected first edit line 2, got %d", results[0].Line)
		}
	})

	t.Run("missing old_str returns STRING_NOT_FOUND", func(t *testing.T) {
		_, _, err := ApplyEditsInMemory("hello\nworld\n", "notes/test.md", []EditSpec{
			{OldStr: "absent", NewStr: "x"},
		})
		svcErr := assertServiceCode(t, err, CodeStringNotFound)
		if svcErr.Details["path"] != "notes/test.md" {
			t.Fatalf("expected details path notes/test.md, got %#v", svcErr.Details)
		}
	})

	t.Run("non-unique old_str returns MULTIPLE_MATCHES", func(t *testing.T) {
		_, _, err := ApplyEditsInMemory("same\nsame\n", "notes/test.md", []EditSpec{
			{OldStr: "same", NewStr: "new"},
		})
		assertServiceCode(t, err, CodeMultipleMatches)
	})
}

func TestAsErrorWithWrappedError(t *testing.T) {
	base := newError(CodeInvalidInput, "bad input", "", nil, nil)
	wrapped := fmt.Errorf("outer: %w", base)

	if got, ok := AsError(base); !ok || got.Code != CodeInvalidInput {
		t.Fatalf("expected AsError to recover editsvc error, got %#v ok=%v", got, ok)
	}
	if got, ok := AsError(wrapped); !ok || got.Code != CodeInvalidInput {
		t.Fatalf("expected AsError to recover wrapped editsvc error, got %#v ok=%v", got, ok)
	}
}
