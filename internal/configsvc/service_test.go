package configsvc

import (
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

func TestSetAndUnsetEditorAndUIFields(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	configPath := filepath.Join(root, "custom", "config.toml")
	statePath := filepath.Join(root, "state", "active.toml")
	vaultPath := filepath.Join(root, "vault")
	if err := config.SaveTo(configPath, &config.Config{
		Vaults: map[string]string{"personal": vaultPath},
	}); err != nil {
		t.Fatalf("save config: %v", err)
	}

	result, err := Set(SetRequest{
		ContextOptions:  ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath},
		Editor:          configStringPtr(" code --wait "),
		EditorMode:      configStringPtr(" GUI "),
		StateFile:       configStringPtr("runtime/state.toml"),
		DefaultVault:    configStringPtr("personal"),
		UIAccent:        configStringPtr(" 39 "),
		UICodeTheme:     configStringPtr(" monokai "),
		UIMarkdownStyle: configStringPtr(" dark "),
	})
	if err != nil {
		t.Fatalf("Set() error = %v", err)
	}
	wantChanged := []string{
		"editor",
		"editor_mode",
		"state_file",
		"default_vault",
		"ui.accent",
		"ui.code_theme",
		"ui.markdown_style",
	}
	if !reflect.DeepEqual(result.Changed, wantChanged) {
		t.Fatalf("Set() Changed = %#v, want %#v", result.Changed, wantChanged)
	}
	if result.Context.ConfigPath != configPath || result.Context.StatePath != statePath {
		t.Fatalf("Set() paths = (%q, %q), want overrides (%q, %q)", result.Context.ConfigPath, result.Context.StatePath, configPath, statePath)
	}

	loaded, err := config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("load set config: %v", err)
	}
	if loaded.Editor != "code --wait" || loaded.EditorMode != "gui" || loaded.StateFile != "runtime/state.toml" || loaded.DefaultVault != "personal" {
		t.Fatalf("set global fields = %#v", loaded)
	}
	if loaded.UI.Accent != "39" || loaded.UI.CodeTheme != "monokai" || loaded.UI.MarkdownStyle != "dark" {
		t.Fatalf("set UI fields = %#v", loaded.UI)
	}

	unset, err := Unset(UnsetRequest{
		ContextOptions:  ContextOptions{ConfigPathOverride: configPath, StatePathOverride: statePath},
		Editor:          true,
		EditorMode:      true,
		StateFile:       true,
		DefaultVault:    true,
		UIAccent:        true,
		UICodeTheme:     true,
		UIMarkdownStyle: true,
	})
	if err != nil {
		t.Fatalf("Unset() error = %v", err)
	}
	if !reflect.DeepEqual(unset.Changed, wantChanged) {
		t.Fatalf("Unset() Changed = %#v, want %#v", unset.Changed, wantChanged)
	}

	loaded, err = config.LoadFrom(configPath)
	if err != nil {
		t.Fatalf("load unset config: %v", err)
	}
	if loaded.Editor != "" || loaded.EditorMode != "" || loaded.StateFile != "" || loaded.DefaultVault != "" || loaded.UI != (config.UIConfig{}) {
		t.Fatalf("Unset() persisted uncleared values: %#v", loaded)
	}
}

func TestSetValidatesEditorAndUIFields(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		set  func(*SetRequest)
		code Code
	}{
		{
			name: "no fields",
			set:  func(*SetRequest) {},
			code: CodeMissingArgument,
		},
		{
			name: "empty editor",
			set:  func(req *SetRequest) { req.Editor = configStringPtr("  ") },
			code: CodeInvalidInput,
		},
		{
			name: "invalid editor mode",
			set:  func(req *SetRequest) { req.EditorMode = configStringPtr("background") },
			code: CodeInvalidInput,
		},
		{
			name: "empty state file",
			set:  func(req *SetRequest) { req.StateFile = configStringPtr("") },
			code: CodeInvalidInput,
		},
		{
			name: "unknown default vault",
			set:  func(req *SetRequest) { req.DefaultVault = configStringPtr("missing") },
			code: CodeInvalidInput,
		},
		{
			name: "empty UI accent",
			set:  func(req *SetRequest) { req.UIAccent = configStringPtr("\t") },
			code: CodeInvalidInput,
		},
		{
			name: "empty UI code theme",
			set:  func(req *SetRequest) { req.UICodeTheme = configStringPtr(" ") },
			code: CodeInvalidInput,
		},
		{
			name: "empty UI markdown style",
			set:  func(req *SetRequest) { req.UIMarkdownStyle = configStringPtr("\n") },
			code: CodeInvalidInput,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			configPath := filepath.Join(t.TempDir(), "config.toml")
			if err := config.SaveTo(configPath, &config.Config{
				Vaults: map[string]string{"personal": "/vault/personal"},
			}); err != nil {
				t.Fatalf("save config: %v", err)
			}
			req := SetRequest{ContextOptions: ContextOptions{ConfigPathOverride: configPath}}
			tt.set(&req)

			_, err := Set(req)
			requireConfigServiceCode(t, err, tt.code)
		})
	}
}

func TestUnsetValidatesSelectionAndConfigPresence(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		createFile bool
		request    UnsetRequest
		code       Code
	}{
		{
			name:       "no selected fields",
			createFile: true,
			request:    UnsetRequest{},
			code:       CodeMissingArgument,
		},
		{
			name:       "missing config file",
			createFile: false,
			request:    UnsetRequest{Editor: true},
			code:       CodeFileNotFound,
		},
	}

	for _, tt := range tests {
		tt := tt
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
				requireConfigServiceCode(t, err, CodeVaultNotFound)
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
				requireConfigServiceCode(t, err, CodeVaultNotFound)
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
		tt := tt
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
		code    Code
	}{
		{
			name: "missing name",
			request: func(_, vaultDir, _ string) VaultAddRequest {
				return VaultAddRequest{RawPath: vaultDir}
			},
			code: CodeMissingArgument,
		},
		{
			name: "missing path",
			request: func(_, _, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "new"}
			},
			code: CodeMissingArgument,
		},
		{
			name: "path does not exist",
			request: func(root, _, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "new", RawPath: filepath.Join(root, "missing")}
			},
			code: CodeFileNotFound,
		},
		{
			name: "path is not directory",
			request: func(_, _, filePath string) VaultAddRequest {
				return VaultAddRequest{Name: "new", RawPath: filePath}
			},
			code: CodeInvalidInput,
		},
		{
			name: "duplicate requires replace",
			request: func(_, vaultDir, _ string) VaultAddRequest {
				return VaultAddRequest{Name: "existing", RawPath: vaultDir}
			},
			code: CodeDuplicateName,
		},
	}

	for _, tt := range tests {
		tt := tt
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

func TestRemoveVaultRequiresConfirmation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		clearDefault bool
		clearActive  bool
		code         Code
		wantRemoved  bool
	}{
		{
			name: "default requires confirmation",
			code: CodeConfirmationNeeded,
		},
		{
			name:         "active also requires confirmation",
			clearDefault: true,
			code:         CodeConfirmationNeeded,
		},
		{
			name:         "confirmed default and active removal",
			clearDefault: true,
			clearActive:  true,
			wantRemoved:  true,
		},
	}

	for _, tt := range tests {
		tt := tt
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

func configStringPtr(value string) *string {
	return &value
}

func requireConfigServiceCode(t *testing.T, err error, want Code) {
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
