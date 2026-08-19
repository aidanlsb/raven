package configsvc

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
	"github.com/aidanlsb/raven/internal/svcerr"
)

func TestSetReturnsSharedServiceError(t *testing.T) {
	t.Parallel()

	_, err := Set(SetRequest{ContextOptions: ContextOptions{
		ConfigPathOverride: filepath.Join(t.TempDir(), "config.toml"),
	}})
	svcErr, ok := svcerr.AsError(err)
	if !ok || svcErr.Code != codes.ErrMissingArgument {
		t.Fatalf("Set() error = %#v, want %s", svcErr, codes.ErrMissingArgument)
	}
}

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

func TestResolveCurrentVaultRejectsMissingActiveVault(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work": "/vault/work",
		},
	}

	current, err := ResolveCurrentVault(cfg, &config.State{ActiveVault: "personal"})
	if err == nil {
		t.Fatalf("ResolveCurrentVault() = %#v, want error", current)
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("ResolveCurrentVault() error = %T %v, want service error", err, err)
	}
	if svcErr.Code != codes.ErrVaultNotFound {
		t.Fatalf("ResolveCurrentVault() code = %q, want %q", svcErr.Code, codes.ErrVaultNotFound)
	}
	if !strings.Contains(svcErr.Message, "active vault 'personal' not found in config") {
		t.Fatalf("ResolveCurrentVault() message = %q, want active vault name", svcErr.Message)
	}
	if !strings.Contains(svcErr.Suggestion, "rvn vault use <name>") || !strings.Contains(svcErr.Suggestion, "rvn vault clear") {
		t.Fatalf("ResolveCurrentVault() suggestion = %q, want use/clear guidance", svcErr.Suggestion)
	}
}

func TestListVaultsIncludesCurrentSelection(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "work",
		Vaults: map[string]string{
			"work":     "/vault/work",
			"personal": "/vault/personal",
		},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:     config.StateVersion,
		ActiveVault: "personal",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	result, err := ListVaults(ContextOptions{
		ConfigPathOverride: configPath,
		StatePathOverride:  statePath,
	})
	if err != nil {
		t.Fatalf("ListVaults() error = %v", err)
	}
	if result.Current == nil {
		t.Fatal("ListVaults() Current = nil, want active vault")
	}
	if result.Current.Name != "personal" || result.Current.Path != "/vault/personal" || result.Current.Source != "active_vault" {
		t.Fatalf("ListVaults() Current = %#v", result.Current)
	}
}

func TestListVaultsKeepsDiagnosticResultWhenCurrentSelectionIsInvalid(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "work",
		Vaults:       map[string]string{"work": "/vault/work"},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:     config.StateVersion,
		ActiveVault: "missing",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}

	result, err := ListVaults(ContextOptions{
		ConfigPathOverride: configPath,
		StatePathOverride:  statePath,
	})
	if err != nil {
		t.Fatalf("ListVaults() error = %v", err)
	}
	if !result.ActiveMissing {
		t.Fatal("ListVaults() ActiveMissing = false, want true")
	}
	if result.Current != nil {
		t.Fatalf("ListVaults() Current = %#v, want nil", result.Current)
	}
}

func TestAddVaultPersistsLoadedSinglePathMigration(t *testing.T) {
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
	if err := os.WriteFile(configPath, []byte(fmt.Sprintf("vault = %q\n", legacyPath)), 0o644); err != nil {
		t.Fatalf("write old config: %v", err)
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

func TestSetAndUnsetGlobalFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "custom", "config.toml")
	statePath := filepath.Join(root, "state", "active.toml")
	vaultPath := filepath.Join(root, "vault")
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "personal",
		Vaults:       map[string]string{"personal": vaultPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	result, err := Set(SetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath},
		Settings: []string{
			"editor= code --wait ",
			"editor_mode= GUI ",
			"ui.markdown_style= dark ",
		},
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	wantSetChanged := []string{
		"editor",
		"editor_mode",
		"ui.markdown_style",
	}
	if !reflect.DeepEqual(result.Changed, wantSetChanged) {
		t.Fatalf("Set() Changed = %#v, want %#v", result.Changed, wantSetChanged)
	}
	if result.Context.ConfigPath != configPath || result.Context.StatePath != statePath {
		t.Fatalf("Set() paths = (%q, %q), want overrides (%q, %q)", result.Context.ConfigPath, result.Context.StatePath, configPath, statePath)
	}

	loaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("load set config: %v", err)
	}
	if loaded.Editor != "code --wait" || loaded.EditorMode != "gui" || loaded.DefaultVault != "personal" {
		t.Fatalf("set global fields = %#v", loaded)
	}
	if loaded.UI.MarkdownStyle != "dark" {
		t.Fatalf("set UI fields = %#v", loaded.UI)
	}

	unset, err := Unset(UnsetRequest{
		ContextOptions: ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath},
		Keys:           []string{"editor", "editor_mode", "default_vault", "ui.markdown_style"},
	})
	if err != nil {
		t.Fatalf("Unset() error = %v", err)
	}
	wantUnsetChanged := []string{"editor", "editor_mode", "default_vault", "ui.markdown_style"}
	if !reflect.DeepEqual(unset.Changed, wantUnsetChanged) {
		t.Fatalf("Unset() Changed = %#v, want %#v", unset.Changed, wantUnsetChanged)
	}

	loaded, err = config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("load unset config: %v", err)
	}
	if loaded.Editor != "" || loaded.EditorMode != "" || loaded.DefaultVault != "" || loaded.UI != (config.UIConfig{}) {
		t.Fatalf("Unset() persisted uncleared values: %#v", loaded)
	}
}

func TestSetValidatesSettings(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		settings []string
		code     codes.ErrorCode
	}{
		{
			name: "no fields",
			code: codes.ErrMissingArgument,
		},
		{
			name:     "empty editor",
			settings: []string{"editor=  "},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "invalid editor mode",
			settings: []string{"editor_mode=background"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "default vault must use pin",
			settings: []string{"default_vault=personal"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "empty UI markdown style",
			settings: []string{"ui.markdown_style=\n"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "unknown key",
			settings: []string{"unknown=value"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "removed single path key",
			settings: []string{"vault=/path/to/notes"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "removed state file key",
			settings: []string{"state_file=runtime/state.toml"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "malformed setting",
			settings: []string{"editor"},
			code:     codes.ErrInvalidInput,
		},
		{
			name:     "duplicate key",
			settings: []string{"editor=nvim", "editor=code"},
			code:     codes.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configPath := filepath.Join(t.TempDir(), "config.toml")
			if err := config.SaveTo(configPath, &config.Config{
				Vaults: map[string]string{"personal": "/vault/personal"},
			}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			req := SetRequest{
				ContextOptions: ContextOptions{ConfigPathOverride: configPath},
				Settings:       tt.settings,
			}

			_, err := Set(req)
			requireConfigServiceCode(t, err, tt.code)
			if tt.name == "default vault must use pin" && !strings.Contains(err.Error(), "rvn vault pin") {
				t.Fatalf("Set() error = %v, want vault pin guidance", err)
			}
			if tt.name == "unknown key" && !strings.Contains(err.Error(), strings.Join(validSetKeys, ", ")) {
				t.Fatalf("Set() error = %v, want valid key list", err)
			}
			if tt.name == "removed single path key" && !strings.Contains(err.Error(), "rvn vault add") {
				t.Fatalf("Set() error = %v, want vault registry guidance", err)
			}
		})
	}
}

func TestUnsetValidatesSelectionAndConfigPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createFile bool
		request    UnsetRequest
		code       codes.ErrorCode
	}{
		{
			name:       "no selected fields",
			createFile: true,
			request:    UnsetRequest{},
			code:       codes.ErrMissingArgument,
		},
		{
			name:       "missing config file",
			createFile: false,
			request:    UnsetRequest{Keys: []string{"editor"}},
			code:       codes.ErrFileNotFound,
		},
		{
			name:       "removed state file key",
			createFile: true,
			request:    UnsetRequest{Keys: []string{"state_file"}},
			code:       codes.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configPath := filepath.Join(t.TempDir(), "nested", "config.toml")
			if tt.createFile {
				if err := config.SaveTo(configPath, &config.Config{Editor: "code"}); err != nil {
					t.Fatalf("save config: %v", err)
				}
			}
			tt.request.ContextOptions.ConfigPathOverride = configPath

			_, err := Unset(tt.request)
			requireConfigServiceCode(t, err, tt.code)
		})
	}
}

func TestConfigPathOverrides(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "custom", "global.toml")
	statePath := filepath.Join(root, "custom-state", "state.toml")
	opts := ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath}

	ctx, err := ShowContext(opts)
	if err != nil {
		t.Fatalf("ShowContext() missing override error = %v", err)
	}
	if ctx.ConfigExists || ctx.ConfigPath != configPath || ctx.StatePath != statePath {
		t.Fatalf("ShowContext() = %#v, want missing override paths", ctx)
	}
	data := ctx.Data()
	if data["editor_mode"] != "auto" || data["state_path"] != statePath {
		t.Fatalf("ShowContext().Data() defaults = %#v", data)
	}
	if _, exists := data["state_file"]; exists {
		t.Fatalf("ShowContext().Data() exposed removed state_file: %#v", data)
	}
	uiData, ok := data["ui"].(map[string]interface{})
	if !ok || uiData["markdown_style"] != "auto" || len(uiData) != 1 {
		t.Fatalf("ShowContext().Data().ui = %#v, want only effective markdown_style=auto", data["ui"])
	}

	initResult, err := Init(InitRequest{ConfigPathOverride: configPath})
	if err != nil {
		t.Fatalf("Init() override error = %v", err)
	}
	if !initResult.Created || initResult.ConfigPath != configPath {
		t.Fatalf("Init() = %#v, want created override", initResult)
	}

	initResult, err = Init(InitRequest{ConfigPathOverride: configPath})
	if err != nil {
		t.Fatalf("second Init() override error = %v", err)
	}
	if initResult.Created {
		t.Fatalf("second Init() Created = true, want false")
	}

	ctx, err = ShowContext(opts)
	if err != nil {
		t.Fatalf("ShowContext() existing override error = %v", err)
	}
	if !ctx.ConfigExists || ctx.ConfigPath != configPath || ctx.StatePath != statePath {
		t.Fatalf("ShowContext() existing = %#v, want override paths", ctx)
	}
}

func TestVaultSelectionLifecycle(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		run  func(*testing.T, ContextOptions, string, string)
	}{
		{
			name: "use persists trimmed active vault",
			run: func(t *testing.T, opts ContextOptions, configPath, statePath string) {
				result, err := UseVault(opts, " personal ")
				if err != nil {
					t.Fatalf("UseVault() error = %v", err)
				}
				if result.ActiveVault != "personal" || result.Path != "/vault/personal" || result.StatePath != statePath {
					t.Fatalf("UseVault() = %#v", result)
				}
				state, err := config.LoadState(statePath)
				if err != nil {
					t.Fatalf("load state: %v", err)
				}
				if state.ActiveVault != "personal" {
					t.Fatalf("active vault = %q, want personal", state.ActiveVault)
				}
			},
		},
		{
			name: "use rejects unknown vault",
			run: func(t *testing.T, opts ContextOptions, _, _ string) {
				_, err := UseVault(opts, "missing")
				requireConfigServiceCode(t, err, codes.ErrVaultNotFound)
			},
		},
		{
			name: "pin persists trimmed default vault",
			run: func(t *testing.T, opts ContextOptions, configPath, _ string) {
				result, err := PinVault(opts, " work ")
				if err != nil {
					t.Fatalf("PinVault() error = %v", err)
				}
				if result.DefaultVault != "work" || result.Path != "/vault/work" || result.ConfigPath != configPath {
					t.Fatalf("PinVault() = %#v", result)
				}
				cfg, err := config.LoadFrom(configPath)
				if err != nil {
					t.Fatalf("load config: %v", err)
				}
				if cfg.DefaultVault != "work" {
					t.Fatalf("default vault = %q, want work", cfg.DefaultVault)
				}
			},
		},
		{
			name: "pin rejects unknown vault",
			run: func(t *testing.T, opts ContextOptions, _, _ string) {
				_, err := PinVault(opts, "missing")
				requireConfigServiceCode(t, err, codes.ErrVaultNotFound)
			},
		},
		{
			name: "clear persists empty active vault",
			run: func(t *testing.T, opts ContextOptions, _, statePath string) {
				result, err := ClearActiveVault(opts)
				if err != nil {
					t.Fatalf("ClearActiveVault() error = %v", err)
				}
				if !result.Cleared || result.Previous != "work" || result.StatePath != statePath {
					t.Fatalf("ClearActiveVault() = %#v", result)
				}
				state, err := config.LoadState(statePath)
				if err != nil {
					t.Fatalf("load state: %v", err)
				}
				if state.ActiveVault != "" {
					t.Fatalf("active vault = %q, want empty", state.ActiveVault)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configPath := filepath.Join(root, "global.toml")
			statePath := filepath.Join(root, "state", "active.toml")
			if err := config.SaveTo(configPath, &config.Config{
				DefaultVault: "personal",
				Vaults: map[string]string{
					"personal": "/vault/personal",
					"work":     "/vault/work",
				},
			}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			if err := config.SaveState(statePath, &config.State{ActiveVault: "work"}); err != nil {
				t.Fatalf("save state: %v", err)
			}

			tt.run(t, ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath}, configPath, statePath)
		})
	}
}

func TestAddVaultValidationAndReplacement(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		request func(root, vaultDir, filePath string) VaultAddRequest
		code    codes.ErrorCode
	}{
		{
			name: "missing name",
			request: func(_, vaultDir, _ string) VaultAddRequest {
				return VaultAddRequest{RawPath: vaultDir}
			},
			code: codes.ErrMissingArgument,
		},
		{
			name: "missing path",
			request: func(_, _, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "new"}
			},
			code: codes.ErrMissingArgument,
		},
		{
			name: "path does not exist",
			request: func(root, _, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "new", RawPath: filepath.Join(root, "missing")}
			},
			code: codes.ErrFileNotFound,
		},
		{
			name: "path is not directory",
			request: func(_, _, filePath string) VaultAddRequest {
				return VaultAddRequest{Name: "new", RawPath: filePath}
			},
			code: codes.ErrInvalidInput,
		},
		{
			name: "duplicate requires replace",
			request: func(_, vaultDir, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "existing", RawPath: vaultDir}
			},
			code: codes.ErrDuplicateName,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configPath := filepath.Join(root, "config.toml")
			vaultDir := filepath.Join(root, "vault")
			if err := os.MkdirAll(vaultDir, 0o755); err != nil {
				t.Fatalf("mkdir vault: %v", err)
			}
			filePath := filepath.Join(root, "not-a-vault")
			if err := os.WriteFile(filePath, []byte("x"), 0o644); err != nil {
				t.Fatalf("write file: %v", err)
			}
			if err := config.SaveTo(configPath, &config.Config{
				Vaults: map[string]string{"existing": filepath.Join(root, "old")},
			}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			req := tt.request(root, vaultDir, filePath)
			req.ConfigPathOverride = configPath

			_, err := AddVault(req)
			requireConfigServiceCode(t, err, tt.code)
		})
	}

	t.Run("replace and pin", func(t *testing.T) {
		t.Parallel()
		root := t.TempDir()
		configPath := filepath.Join(root, "config.toml")
		oldPath := filepath.Join(root, "old")
		newPath := filepath.Join(root, "new")
		if err := os.MkdirAll(newPath, 0o755); err != nil {
			t.Fatalf("mkdir new vault: %v", err)
		}
		if err := config.SaveTo(configPath, &config.Config{
			Vaults: map[string]string{"existing": oldPath},
		}); err != nil {
			t.Fatalf("save config: %v", err)
		}

		result, err := AddVault(VaultAddRequest{
			ContextOptions: ContextOptions{ConfigPathOverride: configPath},
			Name:           " existing ",
			RawPath:        newPath,
			Replace:        true,
			Pin:            true,
		})
		if err != nil {
			t.Fatalf("AddVault() replace error = %v", err)
		}
		if !result.Replaced || !result.Pinned || result.PreviousPath != oldPath || result.DefaultVault != "existing" {
			t.Fatalf("AddVault() replace = %#v", result)
		}
	})
}

func TestFocusVaultValidatesWithoutWritingConfigOrState(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	statePath := filepath.Join(root, "state.toml")
	vaultPath := filepath.Join(root, "work")
	if err := os.MkdirAll(vaultPath, 0o755); err != nil {
		t.Fatalf("mkdir vault: %v", err)
	}
	if err := os.WriteFile(filepath.Join(vaultPath, "raven.yaml"), []byte("{}\n"), 0o644); err != nil {
		t.Fatalf("write vault marker: %v", err)
	}
	if err := config.SaveTo(configPath, &config.Config{
		DefaultVault: "work",
		Vaults:       map[string]string{"work": vaultPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	if err := config.SaveState(statePath, &config.State{
		Version:     config.StateVersion,
		ActiveVault: "work",
	}); err != nil {
		t.Fatalf("save state: %v", err)
	}
	configBefore, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}
	stateBefore, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}

	opts := ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath}
	byName, err := FocusVault(VaultFocusRequest{ContextOptions: opts, Name: "work"})
	if err != nil {
		t.Fatalf("focus by name: %v", err)
	}
	if byName.Name != "work" || byName.Path != vaultPath || byName.Cleared {
		t.Fatalf("focus by name = %#v", byName)
	}

	byPath, err := FocusVault(VaultFocusRequest{ContextOptions: opts, Path: vaultPath})
	if err != nil {
		t.Fatalf("focus by path: %v", err)
	}
	if byPath.Name != "" || byPath.Path != vaultPath || byPath.Cleared {
		t.Fatalf("focus by path = %#v", byPath)
	}

	cleared, err := FocusVault(VaultFocusRequest{ContextOptions: opts, Clear: true})
	if err != nil {
		t.Fatalf("clear focus: %v", err)
	}
	if !cleared.Cleared || cleared.Name != "" || cleared.Path != "" {
		t.Fatalf("clear focus = %#v", cleared)
	}

	configAfter, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("read config after focus: %v", err)
	}
	stateAfter, err := os.ReadFile(statePath)
	if err != nil {
		t.Fatalf("read state after focus: %v", err)
	}
	if string(configAfter) != string(configBefore) {
		t.Fatal("focus changed config.toml")
	}
	if string(stateAfter) != string(stateBefore) {
		t.Fatal("focus changed state.toml")
	}
}

func TestFocusVaultRejectsInvalidTargets(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "config.toml")
	plainDir := filepath.Join(root, "plain")
	if err := os.MkdirAll(plainDir, 0o755); err != nil {
		t.Fatalf("mkdir plain dir: %v", err)
	}
	if err := config.SaveTo(configPath, &config.Config{
		Vaults: map[string]string{"plain": plainDir},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}
	opts := ContextOptions{ConfigPathOverride: configPath}

	tests := []struct {
		name string
		req  VaultFocusRequest
		code codes.ErrorCode
	}{
		{name: "missing mode", req: VaultFocusRequest{}, code: codes.ErrMissingArgument},
		{name: "name and path", req: VaultFocusRequest{Name: "plain", Path: plainDir}, code: codes.ErrInvalidInput},
		{name: "clear and name", req: VaultFocusRequest{Name: "plain", Clear: true}, code: codes.ErrInvalidInput},
		{name: "relative path", req: VaultFocusRequest{Path: "relative"}, code: codes.ErrInvalidInput},
		{name: "missing path", req: VaultFocusRequest{Path: filepath.Join(root, "missing")}, code: codes.ErrFileNotFound},
		{name: "directory without marker", req: VaultFocusRequest{Path: plainDir}, code: codes.ErrInvalidInput},
		{name: "configured directory without marker", req: VaultFocusRequest{Name: "plain"}, code: codes.ErrInvalidInput},
		{name: "unknown name", req: VaultFocusRequest{Name: "missing"}, code: codes.ErrVaultNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.req
			req.ContextOptions = opts
			_, err := FocusVault(req)
			requireConfigServiceCode(t, err, tt.code)
		})
	}
}

func TestRemoveVaultRequiresConfirmation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clearDefault bool
		clearActive  bool
		code         codes.ErrorCode
		wantRemoved  bool
	}{
		{
			name: "default requires confirmation",
			code: codes.ErrConfirmationRequired,
		},
		{
			name:         "active also requires confirmation",
			clearDefault: true,
			code:         codes.ErrConfirmationRequired,
		},
		{
			name:         "confirmed default and active removal",
			clearDefault: true,
			clearActive:  true,
			wantRemoved:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			root := t.TempDir()
			configPath := filepath.Join(root, "config.toml")
			statePath := filepath.Join(root, "state.toml")
			vaultPath := filepath.Join(root, "work")
			if err := config.SaveTo(configPath, &config.Config{
				DefaultVault: "work",
				Vaults:       map[string]string{"work": vaultPath, "personal": filepath.Join(root, "personal")},
			}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			if err := config.SaveState(statePath, &config.State{ActiveVault: "work"}); err != nil {
				t.Fatalf("save state: %v", err)
			}

			result, err := RemoveVault(VaultRemoveRequest{
				ContextOptions: ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath},
				Name:           " work ",
				ClearDefault:   tt.clearDefault,
				ClearActive:    tt.clearActive,
			})
			if !tt.wantRemoved {
				requireConfigServiceCode(t, err, tt.code)
				cfg, loadErr := config.LoadFrom(configPath)
				if loadErr != nil {
					t.Fatalf("load config after rejected removal: %v", loadErr)
				}
				if cfg.Vaults["work"] != vaultPath || cfg.DefaultVault != "work" {
					t.Fatalf("rejected removal changed config: %#v", cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("RemoveVault() error = %v", err)
			}
			if !result.DefaultCleared || !result.ActiveCleared || result.RemovedPath != vaultPath {
				t.Fatalf("RemoveVault() = %#v", result)
			}
			cfg, loadErr := config.LoadFrom(configPath)
			if loadErr != nil {
				t.Fatalf("load config: %v", loadErr)
			}
			if _, exists := cfg.Vaults["work"]; exists || cfg.DefaultVault != "" {
				t.Fatalf("confirmed removal persisted config = %#v", cfg)
			}
			state, loadErr := config.LoadState(statePath)
			if loadErr != nil {
				t.Fatalf("load state: %v", loadErr)
			}
			if state.ActiveVault != "" {
				t.Fatalf("confirmed removal active vault = %q, want empty", state.ActiveVault)
			}
		})
	}
}

func requireConfigServiceCode(t *testing.T, err error, want codes.ErrorCode) {
	t.Helper()
	if err == nil {
		t.Fatalf("error = nil, want %s", want)
	}
	svcErr, ok := svcerr.AsError(err)
	if !ok {
		t.Fatalf("error = %T %v, want service error %s", err, err, want)
	}
	if svcErr.Code != want {
		t.Fatalf("error code = %s, want %s (error: %v)", svcErr.Code, want, err)
	}
	if strings.TrimSpace(svcErr.Message) == "" && svcErr.Err == nil {
		t.Fatalf("service error %s has neither message nor cause", want)
	}
}
