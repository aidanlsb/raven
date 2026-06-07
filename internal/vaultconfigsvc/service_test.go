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

func TestDirectoriesSetNormalizesAndUnsetCompactsConfig(t *testing.T) {
	tmp := t.TempDir()

	setResult, err := SetDirectories(SetDirectoriesRequest{
		VaultPath: tmp,
		Daily:     strPtr("./journal"),
		Object:    strPtr("objects"),
		Template:  strPtr("templates/custom"),
	})
	if err != nil {
		t.Fatalf("SetDirectories() error = %v", err)
	}
	if !setResult.Changed {
		t.Fatalf("expected Changed=true")
	}
	if setResult.Directories.Daily != "journal/" {
		t.Fatalf("expected daily journal/, got %q", setResult.Directories.Daily)
	}

	cfg, err := config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	if cfg.Directories == nil || cfg.Directories.Daily != "journal/" || cfg.Directories.Object != "objects/" || cfg.Directories.Template != "templates/custom/" {
		t.Fatalf("unexpected directories config: %#v", cfg.Directories)
	}

	unsetResult, err := UnsetDirectories(UnsetDirectoriesRequest{
		VaultPath: tmp,
		Daily:     true,
		Object:    true,
		Template:  true,
	})
	if err != nil {
		t.Fatalf("UnsetDirectories() error = %v", err)
	}
	if !unsetResult.Changed {
		t.Fatalf("expected Changed=true")
	}

	cfg, err = config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	if cfg.Directories != nil {
		t.Fatalf("expected directories block cleared, got %#v", cfg.Directories)
	}
}

func TestCaptureSetAndUnsetLifecycle(t *testing.T) {
	tmp := t.TempDir()

	setResult, err := SetCapture(SetCaptureRequest{
		VaultPath:   tmp,
		Destination: strPtr("inbox.md"),
		Heading:     strPtr("## Captured"),
	})
	if err != nil {
		t.Fatalf("SetCapture() error = %v", err)
	}
	if !setResult.Configured {
		t.Fatalf("expected capture configured")
	}
	if setResult.Capture.Destination != "inbox.md" {
		t.Fatalf("expected inbox.md destination, got %q", setResult.Capture.Destination)
	}

	unsetResult, err := UnsetCapture(UnsetCaptureRequest{
		VaultPath:   tmp,
		Destination: true,
		Heading:     true,
	})
	if err != nil {
		t.Fatalf("UnsetCapture() error = %v", err)
	}
	if unsetResult.Configured {
		t.Fatalf("expected capture block cleared")
	}
	if unsetResult.Capture.Destination != "daily" {
		t.Fatalf("expected default daily destination, got %q", unsetResult.Capture.Destination)
	}
}

func TestDeletionSetNormalizesTrashDirAndRejectsInvalidBehavior(t *testing.T) {
	tmp := t.TempDir()

	setResult, err := SetDeletion(SetDeletionRequest{
		VaultPath: tmp,
		Behavior:  strPtr("trash"),
		TrashDir:  strPtr("./archive//trash"),
	})
	if err != nil {
		t.Fatalf("SetDeletion() error = %v", err)
	}
	if setResult.Deletion.TrashDir != "archive/trash" {
		t.Fatalf("expected archive/trash, got %q", setResult.Deletion.TrashDir)
	}

	_, err = SetDeletion(SetDeletionRequest{
		VaultPath: tmp,
		Behavior:  strPtr("invalid"),
	})
	if err == nil {
		t.Fatalf("expected invalid behavior error")
	}
	svcErr, ok := AsError(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if svcErr.Code != CodeInvalidInput {
		t.Fatalf("expected CodeInvalidInput, got %q", svcErr.Code)
	}
}

func strPtr(value string) *string {
	return &value
}
