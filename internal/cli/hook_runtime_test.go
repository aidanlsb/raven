package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/aidanlsb/raven/internal/config"
)

func TestResolveHooksForCommand(t *testing.T) {
	cfg := &config.VaultConfig{
		Hooks: map[string]string{
			"validate": "rvn check --strict",
			"sync":     "git add -A",
		},
		Triggers: map[string]config.HookRefList{
			"after:edit":  {"validate", "sync"},
			"after:*":     {"validate"},
			"after:bogus": {"sync"},
		},
	}

	hooks, warnings := resolveHooksForCommand(cfg, "edit")
	if len(hooks) != 2 || hooks[0] != "validate" || hooks[1] != "sync" {
		t.Fatalf("unexpected hook order: %#v", hooks)
	}

	foundUnknownTrigger := false
	for _, w := range warnings {
		if strings.Contains(w, "unknown trigger command") {
			foundUnknownTrigger = true
			break
		}
	}
	if !foundUnknownTrigger {
		t.Fatalf("expected warning for unknown trigger command, got: %#v", warnings)
	}
}

func TestMutationApplied(t *testing.T) {
	t.Run("bulk preview does not apply", func(t *testing.T) {
		cmd := &cobra.Command{Use: "set"}
		cmd.Flags().Bool("stdin", false, "")
		cmd.Flags().Bool("confirm", false, "")
		_ = cmd.Flags().Set("stdin", "true")
		_ = cmd.Flags().Set("confirm", "false")
		if mutationApplied(cmd, "set") {
			t.Fatal("expected mutationApplied=false for stdin preview mode")
		}
	})

	t.Run("bulk confirm applies", func(t *testing.T) {
		cmd := &cobra.Command{Use: "set"}
		cmd.Flags().Bool("stdin", false, "")
		cmd.Flags().Bool("confirm", false, "")
		_ = cmd.Flags().Set("stdin", "true")
		_ = cmd.Flags().Set("confirm", "true")
		if !mutationApplied(cmd, "set") {
			t.Fatal("expected mutationApplied=true for stdin confirm mode")
		}
	})

	t.Run("import dry-run does not apply", func(t *testing.T) {
		cmd := &cobra.Command{Use: "import"}
		cmd.Flags().Bool("dry-run", false, "")
		_ = cmd.Flags().Set("dry-run", "true")
		if mutationApplied(cmd, "import") {
			t.Fatal("expected mutationApplied=false for import --dry-run")
		}
	})
}

func TestRunNamedHook(t *testing.T) {
	vaultPath := t.TempDir()

	t.Run("success", func(t *testing.T) {
		cfg := &config.VaultConfig{
			Hooks: map[string]string{
				"ok": "echo hello",
			},
			HooksTimeoutSeconds: 5,
		}
		result, err := runNamedHook(vaultPath, cfg, "ok", "manual", 0)
		if err != nil {
			t.Fatalf("runNamedHook returned error: %v", err)
		}
		if result.ExitCode != 0 {
			t.Fatalf("expected exit code 0, got %d", result.ExitCode)
		}
		if result.Stdout != "hello" {
			t.Fatalf("expected stdout hello, got %q", result.Stdout)
		}
	})

	t.Run("failure", func(t *testing.T) {
		cfg := &config.VaultConfig{
			Hooks: map[string]string{
				"bad": "exit 7",
			},
			HooksTimeoutSeconds: 5,
		}
		result, err := runNamedHook(vaultPath, cfg, "bad", "manual", 0)
		if err == nil {
			t.Fatal("expected error for non-zero exit")
		}
		if result.ExitCode != 7 {
			t.Fatalf("expected exit code 7, got %d", result.ExitCode)
		}
	})
}
