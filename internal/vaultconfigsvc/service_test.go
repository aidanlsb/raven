package vaultconfigsvc

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestShowMissingConfigUsesDefaults(t *testing.T) {
	tmp := t.TempDir()

	result, err := Show(ShowRequest{VaultPath: tmp})
	if err != nil {
		t.Fatalf("Show() error = %v", err)
	}

	if result.Exists {
		t.Fatalf("expected Exists=false")
	}
	if !result.AutoReindex {
		t.Fatalf("expected default auto_reindex=true")
	}
	if result.AutoReindexExplicit {
		t.Fatalf("expected AutoReindexExplicit=false")
	}
	if len(result.ProtectedPrefixes) != 0 {
		t.Fatalf("expected no protected prefixes, got %#v", result.ProtectedPrefixes)
	}
}

func TestSetAutoReindexCreatesExplicitValue(t *testing.T) {
	tmp := t.TempDir()

	result, err := SetAutoReindex(SetAutoReindexRequest{
		VaultPath: tmp,
		Value:     false,
	})
	if err != nil {
		t.Fatalf("SetAutoReindex() error = %v", err)
	}
	if !result.Created {
		t.Fatalf("expected Created=true")
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	if result.AutoReindex {
		t.Fatalf("expected AutoReindex=false")
	}

	cfg, err := config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	if cfg.AutoReindex == nil || *cfg.AutoReindex {
		t.Fatalf("expected explicit auto_reindex=false, got %#v", cfg.AutoReindex)
	}
}

func TestUnsetAutoReindexClearsExplicitValue(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "raven.yaml"), []byte("auto_reindex: false\n"), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	result, err := UnsetAutoReindex(UnsetAutoReindexRequest{VaultPath: tmp})
	if err != nil {
		t.Fatalf("UnsetAutoReindex() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	if !result.AutoReindex {
		t.Fatalf("expected AutoReindex=true after unset")
	}
	if result.AutoReindexExplicit {
		t.Fatalf("expected AutoReindexExplicit=false")
	}

	cfg, err := config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	if cfg.AutoReindex != nil {
		t.Fatalf("expected auto_reindex to be cleared, got %#v", cfg.AutoReindex)
	}
}

func TestProtectedPrefixesAddNormalizesAndDeduplicates(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "raven.yaml"), []byte("protected_prefixes:\n  - private/\n"), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	result, err := AddProtectedPrefix(AddProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "./notes//team",
	})
	if err != nil {
		t.Fatalf("AddProtectedPrefix() error = %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	if result.Prefix != "notes/team/" {
		t.Fatalf("expected normalized prefix notes/team/, got %q", result.Prefix)
	}

	result, err = AddProtectedPrefix(AddProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "private",
	})
	if err != nil {
		t.Fatalf("AddProtectedPrefix() duplicate error = %v", err)
	}
	if result.Changed {
		t.Fatalf("expected duplicate add to be unchanged")
	}

	cfg, err := config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	got := normalizedProtectedPrefixes(cfg.ProtectedPrefixes)
	want := []string{"notes/team/", "private/"}
	if len(got) != len(want) {
		t.Fatalf("expected %d prefixes, got %#v", len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("prefix[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProtectedPrefixesRemoveRequiresExistingPrefix(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "raven.yaml"), []byte("protected_prefixes:\n  - private/\n"), 0o644); err != nil {
		t.Fatalf("write raven.yaml: %v", err)
	}

	result, err := RemoveProtectedPrefix(RemoveProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "private",
	})
	if err != nil {
		t.Fatalf("RemoveProtectedPrefix() error = %v", err)
	}
	if result.Removed != "private/" {
		t.Fatalf("expected removed private/, got %q", result.Removed)
	}

	_, err = RemoveProtectedPrefix(RemoveProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "missing",
	})
	if err == nil {
		t.Fatalf("expected missing prefix error")
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != CodePrefixNotFound {
		t.Fatalf("expected CodePrefixNotFound, got %q", svcErr.Code)
	}
}

func TestProtectedPrefixesRejectInvalidPrefix(t *testing.T) {
	tmp := t.TempDir()

	_, err := AddProtectedPrefix(AddProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "../outside",
	})
	if err == nil {
		t.Fatalf("expected invalid prefix error")
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != CodeInvalidInput {
		t.Fatalf("expected CodeInvalidInput, got %q", svcErr.Code)
	}
}
