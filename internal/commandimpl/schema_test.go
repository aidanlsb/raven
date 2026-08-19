package commandimpl

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
	"github.com/aidanlsb/raven/internal/testutil"
)

func TestHandleSchemaUpdateFieldRejectsValueRemap(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
	before := vault.ReadFile("schema.yaml")

	result := HandleSchemaUpdateField(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args: map[string]any{
			"type_name":  "project",
			"field_name": "status",
			"values":     []any{"active", "paused", "done", "archived"},
		},
	})
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrInvalidInput {
		t.Fatalf("expected INVALID_INPUT, got: %#v", result)
	}
	if !strings.Contains(result.Error.Suggestion, "schema convert field") {
		t.Fatalf("unexpected suggestion: %q", result.Error.Suggestion)
	}
	if got := vault.ReadFile("schema.yaml"); got != before {
		t.Fatalf("rejected update changed schema.yaml:\n%s", got)
	}
}

func TestHandleSchemaUpdateTraitRejectsTypeChange(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
	before := vault.ReadFile("schema.yaml")

	result := HandleSchemaUpdateTrait(context.Background(), commandexec.Request{
		VaultPath: vault.Path,
		Args: map[string]any{
			"name": "priority",
			"type": "bool",
		},
	})
	if result.OK || result.Error == nil || result.Error.Code != codes.ErrInvalidInput {
		t.Fatalf("expected INVALID_INPUT, got: %#v", result)
	}
	if !strings.Contains(result.Error.Suggestion, "schema convert trait") {
		t.Fatalf("unexpected suggestion: %q", result.Error.Suggestion)
	}
	if got := vault.ReadFile("schema.yaml"); got != before {
		t.Fatalf("rejected update changed schema.yaml:\n%s", got)
	}
}

func TestHandleSchemaValidateIgnoresUnrelatedInvalidVaultConfig(t *testing.T) {
	t.Parallel()

	vault := testutil.NewTestVault(t).WithSchema(testutil.PersonProjectSchema()).Build()
	if err := os.WriteFile(filepath.Join(vault.Path, "raven.yaml"), []byte("directories: [unterminated\n"), 0o644); err != nil {
		t.Fatalf("write invalid raven.yaml: %v", err)
	}

	result := HandleSchemaValidate(context.Background(), commandexec.Request{VaultPath: vault.Path})
	if !result.OK {
		t.Fatalf("schema validate failed on unrelated config error: %#v", result.Error)
	}
}
