package app

import (
	"context"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/commandexec"
)

// testVaultPath is a placeholder vault path used to satisfy the shared
// validateRequest vault check. validateRequest only inspects that the path is
// non-empty, so the actual directory does not need to exist.
const testVaultPath = "/tmp/raven-validate-request-test-vault"

func TestValidateRequestNormalizesConfirmArg(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "delete",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"stdin":      true,
			"references": []interface{}{"note/one"},
			"confirm":    true,
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if !req.Confirm {
		t.Fatal("Confirm = false, want true from normalized args")
	}
	if req.Preview {
		t.Fatal("Preview = true, want false when confirm is true")
	}
}

func TestValidateRequestNormalizesYesArgToConfirm(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "skill_install",
		Args: map[string]any{
			"yes": true,
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if !req.Confirm {
		t.Fatal("Confirm = false, want true from normalized yes arg")
	}
	if req.Preview {
		t.Fatal("Preview = true, want false when yes is true")
	}
}

func TestValidateRequestDefaultsPreviewForSkillInstall(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "skill_install",
		Args:      map[string]any{},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if req.Confirm {
		t.Fatal("Confirm = true, want false")
	}
	if !req.Preview {
		t.Fatal("Preview = false, want true for skill install without yes")
	}
}

func TestValidateRequestDefaultsPreviewForPreviewCommands(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "check_fix",
		VaultPath: testVaultPath,
		Args:      map[string]any{},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if req.Confirm {
		t.Fatal("Confirm = true, want false")
	}
	if !req.Preview {
		t.Fatal("Preview = false, want true for preview-default command")
	}
}

func TestValidateRequestEditAppliesByDefault(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"reference": "note/example",
			"old_str":   "old",
			"new_str":   "new",
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if req.Confirm {
		t.Fatal("Confirm = true, want false")
	}
	if req.Preview {
		t.Fatal("Preview = true, want false: single-object edit applies by default")
	}
}

func TestValidateRequestDryRunForcesPreviewAndOverridesConfirm(t *testing.T) {
	t.Parallel()

	req, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "edit",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"reference": "note/example",
			"old_str":   "old",
			"new_str":   "new",
			"dry-run":   true,
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if !req.Preview {
		t.Fatal("Preview = false, want true when dry-run is set")
	}

	// dry-run wins even if confirm is also present.
	req, result, ok = validateRequest(context.Background(), commandexec.Request{
		CommandID: "delete",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"stdin":      true,
			"references": []interface{}{"note/one"},
			"confirm":    true,
			"dry-run":    true,
		},
	})
	if !ok {
		t.Fatalf("validateRequest failed: %#v", result)
	}
	if !req.Preview || req.Confirm {
		t.Fatalf("dry-run should force Preview and clear Confirm; got Preview=%v Confirm=%v", req.Preview, req.Confirm)
	}
}

func TestValidateRequestDefaultsPreviewForBulkInputsOnly(t *testing.T) {
	t.Parallel()

	single, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "add",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"text": "hello",
		},
	})
	if !ok {
		t.Fatalf("validateRequest single add failed: %#v", result)
	}
	if single.Preview {
		t.Fatal("single add Preview = true, want false")
	}

	bulk, result, ok := validateRequest(context.Background(), commandexec.Request{
		CommandID: "add",
		VaultPath: testVaultPath,
		Args: map[string]any{
			"text":       "hello",
			"stdin":      true,
			"object_ids": []interface{}{"note/one"},
		},
	})
	if !ok {
		t.Fatalf("validateRequest bulk add failed: %#v", result)
	}
	if !bulk.Preview {
		t.Fatal("bulk add Preview = false, want true")
	}
}

// TestValidateRequestRejectsEmptyVaultPathForVaultCommands verifies that the
// shared invoker gate rejects vault-scoped commands whose request omits a
// vault path, so individual handlers no longer need to repeat the same
// defensive check. The check must fire for every RequiresVault command,
// use the stable VAULT_NOT_SPECIFIED error code, and be skipped for the
// small set of commands that do not require a vault (e.g. version).
//
// The commands covered here accept an empty args map (they have no required
// invoke parameters), which isolates the vault gate from the earlier
// argument-validation gate. Argument validation intentionally still runs
// first when the request carries bad args — a separate MCP parity test
// covers that ordering — but tests targeting the vault gate itself must
// exercise it independently.
func TestValidateRequestRejectsEmptyVaultPathForVaultCommands(t *testing.T) {
	t.Parallel()

	vaultRequiringCommands := []string{
		"check",
		"check_fix",
		"daily",
		"date",
		"query_saved_list",
		"reindex",
		"vault_path",
		"vault_stats",
	}

	for _, commandID := range vaultRequiringCommands {
		t.Run(commandID+"/empty", func(t *testing.T) {
			t.Parallel()

			_, result, ok := validateRequest(context.Background(), commandexec.Request{
				CommandID: commandID,
				VaultPath: "",
				Args:      map[string]any{},
			})
			if ok {
				t.Fatalf("expected validateRequest to reject empty vault path for %q, got success", commandID)
			}
			if result.Error == nil {
				t.Fatalf("expected error envelope, got %#v", result)
			}
			if result.Error.Code != codes.ErrVaultNotSpecified {
				t.Fatalf("error code = %q, want %q", result.Error.Code, codes.ErrVaultNotSpecified)
			}
		})

		t.Run(commandID+"/whitespace", func(t *testing.T) {
			t.Parallel()

			_, result, ok := validateRequest(context.Background(), commandexec.Request{
				CommandID: commandID,
				VaultPath: "   \t\n",
				Args:      map[string]any{},
			})
			if ok {
				t.Fatalf("expected validateRequest to reject whitespace vault path for %q, got success", commandID)
			}
			if result.Error == nil || result.Error.Code != codes.ErrVaultNotSpecified {
				t.Fatalf("expected VAULT_NOT_SPECIFIED, got %#v", result)
			}
		})
	}

	// Commands that do not require a vault must not be blocked by the shared
	// gate, even when VaultPath is empty.
	noVaultCommands := []string{"version", "config_show", "docs", "docs_fetch"}
	for _, commandID := range noVaultCommands {
		t.Run(commandID+"/no_vault_ok", func(t *testing.T) {
			t.Parallel()

			_, result, ok := validateRequest(context.Background(), commandexec.Request{
				CommandID: commandID,
				VaultPath: "",
				Args:      map[string]any{},
			})
			if !ok {
				t.Fatalf("validateRequest unexpectedly rejected %q: %#v", commandID, result)
			}
		})
	}
}
