package cli

import (
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
)

func TestShouldPromptForConfirmUsesTTYSeams(t *testing.T) {
	prevJSON := jsonOutput
	prevStdin := interactiveStdinIsTerminal
	prevStdout := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		interactiveStdinIsTerminal = prevStdin
		interactiveStdoutIsTerminal = prevStdout
	})

	jsonOutput = false
	interactiveStdinIsTerminal = func() bool { return true }
	interactiveStdoutIsTerminal = func() bool { return true }
	if !shouldPromptForConfirm() {
		t.Fatal("expected prompt when both stdio streams are TTYs")
	}

	interactiveStdinIsTerminal = func() bool { return false }
	if shouldPromptForConfirm() {
		t.Fatal("expected no prompt without a stdin TTY")
	}

	interactiveStdinIsTerminal = func() bool { return true }
	jsonOutput = true
	if shouldPromptForConfirm() {
		t.Fatal("expected no prompt in --json mode")
	}
}

func TestConfirmDestructiveWithoutTTYReturnsConfirmationRequired(t *testing.T) {
	prevJSON := jsonOutput
	prevStdin := interactiveStdinIsTerminal
	prevStdout := interactiveStdoutIsTerminal
	t.Cleanup(func() {
		jsonOutput = prevJSON
		interactiveStdinIsTerminal = prevStdin
		interactiveStdoutIsTerminal = prevStdout
	})

	jsonOutput = false
	interactiveStdinIsTerminal = func() bool { return false }
	interactiveStdoutIsTerminal = func() bool { return true }

	ok, result := confirmDestructive("Confirm?", "--force")
	if ok {
		t.Fatal("expected confirmDestructive to refuse without a TTY")
	}
	if result.OK || result.Error == nil {
		t.Fatalf("result = %#v, want CONFIRMATION_REQUIRED failure", result)
	}
	if result.Error.Code != codes.ErrConfirmationRequired {
		t.Fatalf("code = %s, want CONFIRMATION_REQUIRED", result.Error.Code)
	}
	if !strings.Contains(result.Error.Suggestion, "--force") {
		t.Fatalf("suggestion = %q, want --force", result.Error.Suggestion)
	}
}
