package vaultconfigsvc

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
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

	result, err := Show(configTestRuntime(t, tmp))
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

	mutation, autoReindex, explicit, err := SetAutoReindex(configTestRuntime(t, tmp), false)
	if err != nil {
		t.Fatalf("SetAutoReindex() error = %v", err)
	}
	if !mutation.Created {
		t.Fatalf("expected Created=true")
	}
	if !mutation.Changed {
		t.Fatalf("expected Changed=true")
	}
	if autoReindex {
		t.Fatalf("expected AutoReindex=false")
	}
	if !explicit {
		t.Fatalf("expected explicit=true")
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

	mutation, autoReindex, explicit, err := UnsetAutoReindex(configTestRuntime(t, tmp))
	if err != nil {
		t.Fatalf("UnsetAutoReindex() error = %v", err)
	}
	if !mutation.Changed {
		t.Fatalf("expected Changed=true")
	}
	if !autoReindex {
		t.Fatalf("expected AutoReindex=true after unset")
	}
	if explicit {
		t.Fatalf("expected explicit=false")
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

	mutation, prefix, _, err := AddProtectedPrefix(configTestRuntime(t, tmp), "./notes//team")
	if err != nil {
		t.Fatalf("AddProtectedPrefix() error = %v", err)
	}
	if !mutation.Changed {
		t.Fatalf("expected Changed=true")
	}
	if prefix != "notes/team/" {
		t.Fatalf("expected normalized prefix notes/team/, got %q", prefix)
	}

	mutation, _, _, err = AddProtectedPrefix(configTestRuntime(t, tmp), "private")
	if err != nil {
		t.Fatalf("AddProtectedPrefix() duplicate error = %v", err)
	}
	if mutation.Changed {
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

	_, removed, _, err := RemoveProtectedPrefix(configTestRuntime(t, tmp), "private")
	if err != nil {
		t.Fatalf("RemoveProtectedPrefix() error = %v", err)
	}
	if removed != "private/" {
		t.Fatalf("expected removed private/, got %q", removed)
	}

	_, _, _, err = RemoveProtectedPrefix(configTestRuntime(t, tmp), "missing")
	if err == nil {
		t.Fatalf("expected missing prefix error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != codes.ErrPrefixNotFound {
		t.Fatalf("expected CodePrefixNotFound, got %q", svcErr.Code)
	}
}

func TestProtectedPrefixesRejectInvalidPrefix(t *testing.T) {
	tmp := t.TempDir()

	_, _, _, err := AddProtectedPrefix(configTestRuntime(t, tmp), "../outside")
	if err == nil {
		t.Fatalf("expected invalid prefix error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed service error, got %T", err)
	}
	if svcErr.Code != codes.ErrInvalidInput {
		t.Fatalf("expected CodeInvalidInput, got %q", svcErr.Code)
	}
}

func TestDirectoriesSetNormalizesAndUnsetCompactsConfig(t *testing.T) {
	tmp := t.TempDir()
	rt := configTestRuntime(t, tmp)

	mutation, directories, err := SetDirectories(rt, strPtr("./journal"), strPtr("objects"), nil, strPtr("templates/custom"))
	if err != nil {
		t.Fatalf("SetDirectories() error = %v", err)
	}
	if !mutation.Changed {
		t.Fatalf("expected Changed=true")
	}
	if directories.Daily != "journal/" {
		t.Fatalf("expected daily journal/, got %q", directories.Daily)
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

	mutation, _, err = UnsetDirectories(rt, true, true, false, true)
	if err != nil {
		t.Fatalf("UnsetDirectories() error = %v", err)
	}
	if !mutation.Changed {
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

	_, configured, capture, err := SetCapture(configTestRuntime(t, tmp), strPtr("inbox.md"), strPtr("## Captured"))
	if err != nil {
		t.Fatalf("SetCapture() error = %v", err)
	}
	if !configured {
		t.Fatalf("expected capture configured")
	}
	if capture.Destination != "inbox.md" {
		t.Fatalf("expected inbox.md destination, got %q", capture.Destination)
	}

	_, configured, capture, err = UnsetCapture(configTestRuntime(t, tmp), true, true)
	if err != nil {
		t.Fatalf("UnsetCapture() error = %v", err)
	}
	if configured {
		t.Fatalf("expected capture block cleared")
	}
	if capture.Destination != "daily" {
		t.Fatalf("expected default daily destination, got %q", capture.Destination)
	}
}

func TestDeletionSetNormalizesTrashDirAndRejectsInvalidBehavior(t *testing.T) {
	tmp := t.TempDir()

	_, _, deletion, err := SetDeletion(configTestRuntime(t, tmp), strPtr("trash"), strPtr("./archive//trash"))
	if err != nil {
		t.Fatalf("SetDeletion() error = %v", err)
	}
	if deletion.TrashDir != "archive/trash" {
		t.Fatalf("expected archive/trash, got %q", deletion.TrashDir)
	}

	_, _, _, err = SetDeletion(configTestRuntime(t, tmp), strPtr("invalid"), nil)
	if err == nil {
		t.Fatalf("expected invalid behavior error")
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("expected typed error, got %T", err)
	}
	if svcErr.Code != codes.ErrInvalidInput {
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

			configPath, exists, patterns, err := ListExclude(configTestRuntime(t, vaultPath))
			if err != nil {
				t.Fatalf("ListExclude() error = %v", err)
			}
			if exists != tt.wantExists || !reflect.DeepEqual(patterns, tt.want) {
				t.Fatalf("ListExclude() = Exists=%v Exclude=%#v, want Exists=%v Exclude=%#v", exists, patterns, tt.wantExists, tt.want)
			}
			if configPath != filepath.Join(vaultPath, "raven.yaml") {
				t.Fatalf("ConfigPath = %q, want vault raven.yaml", configPath)
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
		wantCode    codes.ErrorCode
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
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:     "rejects malformed glob",
			pattern:  "[",
			wantCode: codes.ErrInvalidInput,
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

			mutation, pattern, patterns, err := AddExclude(configTestRuntime(t, vaultPath), tt.pattern)
			if tt.wantCode != "" {
				requireVaultConfigCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("AddExclude() error = %v", err)
			}
			if mutation.Created != tt.wantCreated || mutation.Changed != tt.wantChanged ||
				pattern != tt.wantPattern || !reflect.DeepEqual(patterns, tt.wantExclude) {
				t.Fatalf("AddExclude() = Created=%v Changed=%v Pattern=%q Exclude=%#v, want Created=%v Changed=%v Pattern=%q Exclude=%#v",
					mutation.Created, mutation.Changed, pattern, patterns, tt.wantCreated, tt.wantChanged, tt.wantPattern, tt.wantExclude)
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
		wantCode    codes.ErrorCode
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
			wantCode: codes.ErrPrefixNotFound,
		},
		{
			name:     "invalid pattern is rejected",
			pattern:  "[",
			wantCode: codes.ErrInvalidInput,
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

			mutation, removed, patterns, err := RemoveExclude(configTestRuntime(t, vaultPath), tt.pattern)
			if tt.wantCode != "" {
				requireVaultConfigCode(t, err, tt.wantCode)
				return
			}
			if err != nil {
				t.Fatalf("RemoveExclude() error = %v", err)
			}
			if !mutation.Changed || removed != tt.wantRemoved || !reflect.DeepEqual(patterns, tt.wantExclude) {
				t.Fatalf("RemoveExclude() = Changed=%v Removed=%q Exclude=%#v, want Removed=%q Exclude=%#v", mutation.Changed, removed, patterns, tt.wantRemoved, tt.wantExclude)
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

func requireVaultConfigCode(t *testing.T, err error, want codes.ErrorCode) {
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
