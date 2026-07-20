package configsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestSameVaultPathTreatsSymlinkAsSameVault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	vaultPath := filepath.Join(root, "vault")
	aliasPath := filepath.Join(root, "vault-alias")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.Symlink(vaultPath, aliasPath); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	if !SameVaultPath(aliasPath, vaultPath) {
		t.Fatalf("SameVaultPath(%q, %q) = false, want true", aliasPath, vaultPath)
	}
}

func TestAddVaultPreservesLegacyDefault(t *testing.T) {
	t.Parallel()
	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	legacyPath := filepath.Join(root, "legacy")
	newPath := filepath.Join(root, "new")
	for _, path := range []string{legacyPath, newPath} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatalf("mkdir vault: %v", err)
		}
	}
	if err := config.SaveTo(configPath, &config.Config{Vault: legacyPath}); err != nil {
		t.Fatalf("save legacy config: %v", err)
	}

	if _, err := AddVault(VaultAddRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath},
		Name:           "new",
		RawPath:        newPath,
	}); err != nil {
		t.Fatalf("add vault: %v", err)
	}

	cfg, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("load migrated config: %v", err)
	}
	if cfg.Vault != "" {
		t.Fatalf("legacy vault = %q, want cleared after migration", cfg.Vault)
	}
	if cfg.DefaultVault != "default" {
		t.Fatalf("default_vault = %q, want default", cfg.DefaultVault)
	}
	if cfg.Vaults["default"] != legacyPath || cfg.Vaults["new"] != newPath {
		t.Fatalf("vaults = %#v, want preserved legacy and new vault", cfg.Vaults)
	}
	current, err := ResolveCurrentVault(cfg, &config.State{})
	if err != nil {
		t.Fatalf("resolve migrated default: %v", err)
	}
	if current.Name != "default" || current.Path != legacyPath {
		t.Fatalf("current vault = %#v, want migrated legacy default", current)
	}
}
