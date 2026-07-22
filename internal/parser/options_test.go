package parser

import (
	"testing"

	"github.com/aidanlsb/raven/internal/config"
)

func TestOptionsFromVaultConfig(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		vaultConfig *config.VaultConfig
		wantNil     bool
		wantObjects string
		wantPages   string
		wantDaily   string
	}{
		{
			name:    "nil config",
			wantNil: true,
		},
		{
			name: "without directories config",
			vaultConfig: &config.VaultConfig{
				Directories: &config.DirectoriesConfig{
					Daily:    "journal",
					Template: "templates",
				},
			},
			wantDaily: "journal",
		},
		{
			name: "with directories config",
			vaultConfig: &config.VaultConfig{
				Directories: &config.DirectoriesConfig{
					Object: "objects",
					Page:   "pages",
					Daily:  "journal",
				},
			},
			wantObjects: "objects/",
			wantPages:   "pages/",
			wantDaily:   "journal",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := OptionsFromVaultConfig(tt.vaultConfig)
			if tt.wantNil {
				if got != nil {
					t.Fatalf("OptionsFromVaultConfig() = %#v, want nil", got)
				}
				return
			}
			if got == nil {
				t.Fatal("OptionsFromVaultConfig() = nil, want options")
			}
			if got.ObjectsRoot != tt.wantObjects || got.PagesRoot != tt.wantPages || got.DailyRoot != tt.wantDaily {
				t.Fatalf(
					"OptionsFromVaultConfig() = %#v, want ObjectsRoot=%q PagesRoot=%q DailyRoot=%q",
					got,
					tt.wantObjects,
					tt.wantPages,
					tt.wantDaily,
				)
			}
		})
	}
}
