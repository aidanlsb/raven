package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/commandexec"
)

func requireJSONResponseFailure(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errJSONResponseFailure) {
		t.Fatalf("error = %v, want errJSONResponseFailure", err)
	}
}

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

	requireJSONResponseFailure(t, outputErr)

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

func TestEmitJSONErrorEnvelopeWritesFlagParseFailure(t *testing.T) {
	parseErr := errors.New("unknown shorthand flag: ' ' in - probe a")
	var got error
	out := captureStdout(t, func() {
		got = emitJSONErrorEnvelope(true, parseErr)
	})
	requireJSONResponseFailure(t, got)

	var resp Response
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatalf("unmarshal failure envelope: %v\noutput=%s", err, out)
	}
	if resp.OK || resp.Error == nil || resp.Error.Code != ErrInvalidInput {
		t.Fatalf("error = %#v, want INVALID_INPUT\noutput=%s", resp.Error, out)
	}
	if !strings.Contains(resp.Error.Message, "unknown shorthand flag") {
		t.Fatalf("message = %q, want cobra flag-parse text", resp.Error.Message)
	}
	if !strings.Contains(resp.Error.Suggestion, `--json -- "- Review the rollout"`) {
		t.Fatalf("suggestion = %q, want dash-terminator example", resp.Error.Suggestion)
	}
}

func TestOutputJSONIsCompact(t *testing.T) {
	var outputErr error
	out := captureStdout(t, func() {
		outputErr = outputJSON(commandexec.Success(map[string]string{"ok": "yes"}, nil))
	})
	if outputErr != nil {
		t.Fatalf("outputJSON() error = %v", outputErr)
	}
	if strings.Contains(out, "\n  ") || strings.Contains(out, "\n\t") {
		t.Fatalf("expected compact JSON, got indented output:\n%s", out)
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "{") {
		t.Fatalf("expected JSON object, got %q", out)
	}
}

func TestFlagParseSuggestionUnknownFlag(t *testing.T) {
	got := flagParseSuggestionFor(searchCmd, errors.New("unknown flag: --limt"))
	if !strings.Contains(got, "--limit") {
		t.Fatalf("suggestion = %q, want closest flag --limit", got)
	}
	if !strings.Contains(got, "rvn search --help") {
		t.Fatalf("suggestion = %q, want help line", got)
	}
	if strings.Contains(got, "starts with a dash") {
		t.Fatalf("suggestion = %q, should not mention dash terminator", got)
	}
}

func TestFlagParseSuggestionDashPrefixedValue(t *testing.T) {
	got := flagParseSuggestionFor(addCmd, errors.New("unknown shorthand flag: ' ' in - probe a"))
	if !strings.Contains(got, `--json -- "- Review the rollout"`) {
		t.Fatalf("suggestion = %q, want dash-terminator example", got)
	}
}

func TestEmitJSONErrorEnvelopeLeavesHumanErrorsAlone(t *testing.T) {
	parseErr := errors.New("unknown shorthand flag: ' ' in - probe a")
	var got error
	out := captureStdout(t, func() {
		got = emitJSONErrorEnvelope(false, parseErr)
	})
	if !errors.Is(got, parseErr) {
		t.Fatalf("error = %v, want original cobra error", got)
	}
	if out != "" {
		t.Fatalf("expected no JSON stdout in human mode, got %q", out)
	}
}

func TestEmitJSONErrorEnvelopeSkipsSentinels(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "already written", err: errJSONResponseFailure},
		{name: "pick cancelled", err: ErrPickCancelled},
		{name: "help", err: flag.ErrHelp},
		{name: "nil", err: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var got error
			out := captureStdout(t, func() {
				got = emitJSONErrorEnvelope(true, tc.err)
			})
			if tc.err == nil {
				if got != nil {
					t.Fatalf("error = %v, want nil", got)
				}
			} else if !errors.Is(got, tc.err) {
				t.Fatalf("error = %v, want %v", got, tc.err)
			}
			if out != "" {
				t.Fatalf("expected no extra stdout, got %q", out)
			}
		})
	}
}

func TestArgsRequestJSON(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		args []string
		want bool
	}{
		{name: "bare flag", args: []string{"add", "--json"}, want: true},
		{name: "after other flags", args: []string{"add", "- probe a", "--to", "today", "--json"}, want: true},
		{name: "equals true", args: []string{"--json=true"}, want: true},
		{name: "equals false", args: []string{"--json=false"}, want: false},
		{name: "absent", args: []string{"add", "note"}, want: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := argsRequestJSON(tc.args); got != tc.want {
				t.Fatalf("argsRequestJSON(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}
