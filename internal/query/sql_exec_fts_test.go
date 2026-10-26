package query

import (
	"errors"
	"testing"
)

func TestWrapQueryExecErrorClassifiesFTSSyntax(t *testing.T) {
	t.Parallel()

	err := wrapQueryExecError(errors.New("no such column: type"))
	var execErr *ExecutionError
	if !errors.As(err, &execErr) {
		t.Fatalf("error = %v, want ExecutionError", err)
	}
	if execErr.Message != "invalid full-text search query" {
		t.Fatalf("message = %q", execErr.Message)
	}
}

func TestWrapQueryExecErrorLeavesOrdinaryDatabaseErrors(t *testing.T) {
	t.Parallel()

	err := wrapQueryExecError(errors.New("disk I/O error"))
	var execErr *ExecutionError
	if errors.As(err, &execErr) {
		t.Fatalf("ordinary DB error was classified as ExecutionError: %v", err)
	}
	if err == nil || err.Error() != "query failed: disk I/O error" {
		t.Fatalf("error = %v", err)
	}
}
