package mutationguard

import (
	"path/filepath"
	"testing"

	"github.com/aidanlsb/raven/internal/codes"
	"github.com/aidanlsb/raven/internal/config"
)

func TestValidateContentMutationFilePath(t *testing.T) {
	t.Parallel()

	vaultPath := t.TempDir()
	tests := []struct {
		name      string
		vaultPath string
		vaultCfg  *config.VaultConfig
		filePath  string
		wantCode  codes.ErrorCode
	}{
		{
			name:     "empty path",
			filePath: " \t ",
			wantCode: codes.ErrInvalidInput,
		},
		{
			name:      "absolute allowed path",
			vaultPath: vaultPath,
			vaultCfg:  &config.VaultConfig{},
			filePath:  filepath.Join(vaultPath, "notes", "allowed.md"),
		},
		{
			name:      "absolute protected path",
			vaultPath: vaultPath,
			vaultCfg:  &config.VaultConfig{ProtectedPrefixes: []string{"private/"}},
			filePath:  filepath.Join(vaultPath, "private", "blocked.md"),
			wantCode:  codes.ErrValidationFailed,
		},
		{
			name:     "absolute path without vault path preserves legacy allowance",
			filePath: filepath.Join(vaultPath, "schema.yaml"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateContentMutationFilePath(tt.vaultPath, tt.vaultCfg, tt.filePath)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateContentMutationFilePath() error = %v", err)
				}
				return
			}
			if err == nil || err.Code != tt.wantCode {
				t.Fatalf("ValidateContentMutationFilePath() error = %#v, want code %s", err, tt.wantCode)
			}
		})
	}
}

func TestValidateContentMutationRelPath(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		vaultCfg       *config.VaultConfig
		relPath        string
		wantCode       codes.ErrorCode
		wantMessage    string
		wantSuggestion string
		wantPath       string
		wantCause      bool
	}{
		{
			name:    "allowed path",
			relPath: "notes/allowed.md",
		},
		{
			name:    "blank relative path preserves legacy allowance",
			relPath: " ",
		},
		{
			name:           "hard protected file is normalized",
			relPath:        "./schema.yaml",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "cannot modify protected or system-managed paths",
			wantSuggestion: "Choose a path outside protected prefixes, or update them with 'rvn vault config protected-prefixes ...'",
			wantPath:       "schema.yaml",
		},
		{
			name: "configured protected prefix",
			vaultCfg: &config.VaultConfig{
				ProtectedPrefixes: []string{"private/"},
			},
			relPath:        "private/blocked.md",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "cannot modify protected or system-managed paths",
			wantSuggestion: "Choose a path outside protected prefixes, or update them with 'rvn vault config protected-prefixes ...'",
			wantPath:       "private/blocked.md",
		},
		{
			name: "excluded path",
			vaultCfg: &config.VaultConfig{
				Exclude: []string{"archive/"},
			},
			relPath:        "archive/blocked.md",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "cannot modify excluded paths",
			wantSuggestion: "Choose a managed path, or update exclusions with 'rvn vault config exclude ...'",
			wantPath:       "archive/blocked.md",
		},
		{
			name: "invalid exclude config",
			vaultCfg: &config.VaultConfig{
				Exclude: []string{"["},
			},
			relPath:        "notes/blocked.md",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "invalid exclude config",
			wantSuggestion: "Fix raven.yaml exclude patterns and try again",
			wantPath:       "notes/blocked.md",
			wantCause:      true,
		},
		{
			name:           "default template directory",
			vaultCfg:       &config.VaultConfig{},
			relPath:        "templates/default.md",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "cannot modify template files with content mutation commands",
			wantSuggestion: "Use 'rvn template ...' or 'rvn schema template ...' for template changes",
			wantPath:       "templates/default.md",
		},
		{
			name: "custom template directory",
			vaultCfg: &config.VaultConfig{
				Directories: &config.DirectoriesConfig{Template: "blueprints/"},
			},
			relPath:        "blueprints/default.md",
			wantCode:       codes.ErrValidationFailed,
			wantMessage:    "cannot modify template files with content mutation commands",
			wantSuggestion: "Use 'rvn template ...' or 'rvn schema template ...' for template changes",
			wantPath:       "blueprints/default.md",
		},
		{
			name:     "nil config does not apply template policy",
			relPath:  "templates/default.md",
			vaultCfg: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := ValidateContentMutationRelPath(tt.vaultCfg, tt.relPath)
			if tt.wantCode == "" {
				if err != nil {
					t.Fatalf("ValidateContentMutationRelPath() error = %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("ValidateContentMutationRelPath() error = nil, want code %s", tt.wantCode)
			}
			if err.Code != tt.wantCode {
				t.Errorf("error code = %s, want %s", err.Code, tt.wantCode)
			}
			if err.Message != tt.wantMessage {
				t.Errorf("error message = %q, want %q", err.Message, tt.wantMessage)
			}
			if err.Suggestion != tt.wantSuggestion {
				t.Errorf("error suggestion = %q, want %q", err.Suggestion, tt.wantSuggestion)
			}
			if got, _ := err.Details["path"].(string); got != tt.wantPath {
				t.Errorf("error path detail = %q, want %q", got, tt.wantPath)
			}
			if got := err.Err != nil; got != tt.wantCause {
				t.Errorf("error has cause = %t, want %t", got, tt.wantCause)
			}
		})
	}
}
