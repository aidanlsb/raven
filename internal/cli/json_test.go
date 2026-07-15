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
	t.Parallel()

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
