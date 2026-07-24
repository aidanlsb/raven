package vaultconfigsvc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
	"github.com/aidanlsb/raven/internal/testutil"
	"github.com/aidanlsb/raven/internal/vaultruntime"
)

func configTestRuntime(t *testing.T, vaultPath string) *vaultruntime.Runtime {
	t.Helper()
	return testutil.NewVaultRuntime(t, vaultPath, vaultruntime.Options{SkipSchema: true})
}

func TestShowMissingConfigUsesDefaults(t *testing.T) {
	tmp := t.TempDir()

	result, err := Show(configTestRuntime(t, tmp), ShowRequest{VaultPath: tmp})
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

	result, err := SetAutoReindex(configTestRuntime(t, tmp), SetAutoReindexRequest{
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

	result, err := UnsetAutoReindex(configTestRuntime(t, tmp), UnsetAutoReindexRequest{VaultPath: tmp})
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

	result, err := AddProtectedPrefix(configTestRuntime(t, tmp), AddProtectedPrefixRequest{
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

	result, err = AddProtectedPrefix(configTestRuntime(t, tmp), AddProtectedPrefixRequest{
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

	result, err := RemoveProtectedPrefix(configTestRuntime(t, tmp), RemoveProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "private",
	})
	if err != nil {
		t.Fatalf("RemoveProtectedPrefix() error = %v", err)
	}
	if result.Removed != "private/" {
		t.Fatalf("expected removed private/, got %q", result.Removed)
	}

	_, err = RemoveProtectedPrefix(configTestRuntime(t, tmp), RemoveProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "missing",
	})
	if err == nil {
		t.Fatalf("expected missing prefix error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != CodePrefixNotFound {
		t.Fatalf("expected CodePrefixNotFound, got %q", svcErr.Code)
	}
}

func TestProtectedPrefixesRejectInvalidPrefix(t *testing.T) {
	tmp := t.TempDir()

	_, err := AddProtectedPrefix(configTestRuntime(t, tmp), AddProtectedPrefixRequest{
		VaultPath: tmp,
		Prefix:    "../outside",
	})
	if err == nil {
		t.Fatalf("expected invalid prefix error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != CodeInvalidInput {
		t.Fatalf("expected CodeInvalidInput, got %q", svcErr.Code)
	}
}

func TestDirectoriesSetNormalizesAndUnsetCompactsConfig(t *testing.T) {
	tmp := t.TempDir()
	rt := configTestRuntime(t, tmp)

	setResult, err := SetDirectories(rt, SetDirectoriesRequest{
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
	if !rt.VaultConfigExists || rt.ParseOptions == nil || rt.ParseOptions.DailyRoot != "journal" {
		t.Fatalf("runtime was not refreshed after config save: %#v", rt)
	}

	cfg, err := config.LoadVaultConfig(tmp)
	if err != nil {
		t.Fatalf("LoadVaultConfig() error = %v", err)
	}
	if cfg.Directories == nil || cfg.Directories.Daily != "journal/" || cfg.Directories.Object != "objects/" || cfg.Directories.Template != "templates/custom/" {
		t.Fatalf("unexpected directories config: %#v", cfg.Directories)
	}

	unsetResult, err := UnsetDirectories(rt, UnsetDirectoriesRequest{
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

	setResult, err := SetCapture(configTestRuntime(t, tmp), SetCaptureRequest{
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

	unsetResult, err := UnsetCapture(configTestRuntime(t, tmp), UnsetCaptureRequest{
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

	setResult, err := SetDeletion(configTestRuntime(t, tmp), SetDeletionRequest{
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

	_, err = SetDeletion(configTestRuntime(t, tmp), SetDeletionRequest{
		VaultPath: tmp,
		Behavior:  strPtr("invalid"),
	})
	if err == nil {
		t.Fatalf("expected invalid behavior error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if svcErr.Code != CodeInvalidInput {
		t.Fatalf("expected CodeInvalidInput, got %q", svcErr.Code)
	}
}

func TestExcludeList(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		configYAML string
		wantExists bool
		want       []string
	}{
		{
			name:       "missing config",
			wantExists: false,
			want:       nil,
		},
		{
			name: "normalizes configured patterns",
			configYAML: "exclude:\n" +
				"  - '  *.plan.md  '\n" +
				"  - '.cursor/'\n" +
				"  - '  '\n",
			wantExists: true,
			want:       []string{"*.plan.md", ".cursor/"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			if tt.configYAML != "" {
				if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte(tt.configYAML), 0o644); err != nil {
					t.Fatalf("write raven.yaml: %v", err)
				}
			}

			result, err := ListExclude(configTestRuntime(t, vaultPath), ListExcludeRequest{VaultPath: vaultPath})
			if err != nil {
				t.Fatalf("ListExclude() error = %v", err)
			}
			if result.Exists != tt.wantExists || !reflect.DeepEqual(result.Exclude, tt.want) {
				t.Fatalf("ListExclude() = %#v, want Exists=%v Exclude=%#v", result, tt.wantExists, tt.want)
			}
			if result.ConfigPath != filepath.Join(vaultPath, "raven.yaml") {
				t.Fatalf("ConfigPath = %q, want vault raven.yaml", result.ConfigPath)
			}
		})
	}
}

func TestExcludeAdd(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		configYAML  string
		pattern     string
		wantCode    Code
		wantCreated bool
		wantChanged bool
		wantPattern string
		wantExclude []string
	}{
		{
			name:        "creates config with normalized pattern",
			pattern:     "  *.plan.md  ",
			wantCreated: true,
			wantChanged: true,
			wantPattern: "*.plan.md",
			wantExclude: []string{"*.plan.md"},
		},
		{
			name:        "appends while preserving order",
			configYAML:  "exclude:\n  - '.cursor/'\n",
			pattern:     "*.plan.md",
			wantChanged: true,
			wantPattern: "*.plan.md",
			wantExclude: []string{".cursor/", "*.plan.md"},
		},
		{
			name:        "duplicate is unchanged",
			configYAML:  "exclude:\n  - '*.plan.md'\n",
			pattern:     " *.plan.md ",
			wantChanged: false,
			wantPattern: "*.plan.md",
			wantExclude: []string{"*.plan.md"},
		},
		{
			name:     "rejects empty pattern",
			pattern:  " \n ",
			wantCode: CodeInvalidInput,
		},
		{
			name:     "rejects malformed glob",
			pattern:  "[",
			wantCode: CodeInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			if tt.configYAML != "" {
				if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte(tt.configYAML), 0o644); err != nil {
					t.Fatalf("write raven.yaml: %v", err)
				}
			}

			result, err := AddExclude(configTestRuntime(t, vaultPath), AddExcludeRequest{
				VaultPath: vaultPath,
				Pattern:   tt.pattern,
			})
			if tt.wantCode != "" {
				requireVaultConfigCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("AddExclude() error = %v", err)
			}
			if result.Created != tt.wantCreated || result.Changed != tt.wantChanged ||
				result.Pattern != tt.wantPattern || !reflect.DeepEqual(result.Exclude, tt.wantExclude) {
				t.Fatalf("AddExclude() = %#v, want Created=%v Changed=%v Pattern=%q Exclude=%#v",
					result, tt.wantCreated, tt.wantChanged, tt.wantPattern, tt.wantExclude)
			}

			cfg, err := config.LoadVaultConfig(vaultPath)
			if err != nil {
				t.Fatalf("LoadVaultConfig() error = %v", err)
			}
			if !reflect.DeepEqual(normalizedExcludePatterns(cfg.Exclude), tt.wantExclude) {
				t.Fatalf("persisted exclude = %#v, want %#v", cfg.Exclude, tt.wantExclude)
			}
		})
	}
}

func TestExcludeRemove(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		pattern     string
		wantCode    Code
		wantRemoved string
		wantExclude []string
	}{
		{
			name:        "removes normalized existing pattern",
			pattern:     " *.plan.md ",
			wantRemoved: "*.plan.md",
			wantExclude: []string{".cursor/"},
		},
		{
			name:     "missing pattern returns stable code",
			pattern:  "*.missing.md",
			wantCode: CodePrefixNotFound,
		},
		{
			name:     "invalid pattern is rejected",
			pattern:  "[",
			wantCode: CodeInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			vaultPath := t.TempDir()
			if err := os.WriteFile(
				filepath.Join(vaultPath, "raven.yaml"),
				[]byte("exclude:\n  - '.cursor/'\n  - '*.plan.md'\n"),
				0o644,
			); err != nil {
				t.Fatalf("write raven.yaml: %v", err)
			}

			result, err := RemoveExclude(configTestRuntime(t, vaultPath), RemoveExcludeRequest{
				VaultPath: vaultPath,
				Pattern:   tt.pattern,
			})
			if tt.wantCode != "" {
				requireVaultConfigCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("RemoveExclude() error = %v", err)
			}
			if !result.Changed || result.Removed != tt.wantRemoved || !reflect.DeepEqual(result.Exclude, tt.wantExclude) {
				t.Fatalf("RemoveExclude() = %#v, want Removed=%q Exclude=%#v", result, tt.wantRemoved, tt.wantExclude)
			}

			cfg, err := config.LoadVaultConfig(vaultPath)
			if err != nil {
				t.Fatalf("LoadVaultConfig() error = %v", err)
			}
			if !reflect.DeepEqual(normalizedExcludePatterns(cfg.Exclude), tt.wantExclude) {
				t.Fatalf("persisted exclude = %#v, want %#v", cfg.Exclude, tt.wantExclude)
			}
		})
	}
}

func requireVaultConfigCode(t *testing.T, err error, want Code) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("error = %T %v, want service error", err, err)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %s, want %s", svcErr.Code, want)
	}
}

func strPtr(value string) *string {
	return &value
}
