package commandimpl

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

func TestSavedQueryArgumentValidationPrecedesConfigLoad(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("queries: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write invalid raven.yaml: %v", err)
	}

	result := HandleQuerySavedGet(context.Background(), commandexec.Request{VaultPath: vaultPath})
	if result.Error == nil || result.Error.Code != codes.ErrInvalidInput {
		t.Fatalf("error = %#v, want INVALID_INPUT", result.Error)
	}
}
