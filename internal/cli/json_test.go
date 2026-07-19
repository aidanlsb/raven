package cli

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestOutputJSONPropagatesWriteErrors(t *testing.T) {
	// This test mutates the global os.Stdout, which is shared with the
	// captureStdout helper used by other (parallel) tests. Hold the same mutex
	// so the swap is mutually exclusive with those captures; otherwise a
	// concurrent capture can replace our closed pipe with a working one and the
	// write unexpectedly succeeds (flaky "expected write error, got nil").
	captureStdoutMu.Lock()
	defer captureStdoutMu.Unlock()

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if err := r.Close(); err != nil {
		t.Fatalf("close reader: %v", err)
	}

	prev := os.Stdout
	os.Stdout = w
	defer func() {
		os.Stdout = prev
		_ = w.Close()
	}()

	err = outputJSON(commandexec.Success(map[string]string{"ok": "yes"}, nil))
	if err == nil {
		t.Fatal("expected write error, got nil")
	}
	if !errors.Is(err, io.ErrClosedPipe) && !errors.Is(err, os.ErrClosed) {
		// Encode wraps the underlying write error; ensure message is useful.
		var syntax *json.SyntaxError
		if errors.As(err, &syntax) {
			t.Fatalf("unexpected syntax error: %v", err)
		}
		if err.Error() == "" {
			t.Fatal("expected non-empty error message")
		}
	}
}

func TestOutputJSONReturnsErrorAfterWritingFailureEnvelope(t *testing.T) {
	var outputErr error
	out := captureStdout(t, func() {
		outputErr = outputJSON(commandexec.Failure(ErrInvalidInput, "invalid input", nil, "try again"))
	})

	if !errors.Is(outputErr, errJSONResponseFailure) {
		t.Fatalf("outputJSON() error = %v, want errJSONResponseFailure", outputErr)
	}

	var resp Response
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal failure envelope: %v\noutput=%s", err, out)
	}
	if resp.OK {
		t.Fatalf("expected ok=false\noutput=%s", out)
	}
	if resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("error = %#v, want code %s\noutput=%s", resp.Error, ErrInvalidInput, out)
	}
}
